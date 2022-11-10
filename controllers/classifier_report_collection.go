/*
Copyright 2022. projectsveltos.io. All rights reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	libsveltosv1alpha1 "github.com/projectsveltos/libsveltos/api/v1alpha1"
	"github.com/projectsveltos/libsveltos/lib/clusterproxy"
	logs "github.com/projectsveltos/libsveltos/lib/logsettings"
)

const (
	classifierReportClusterLabel = "projectsveltos.io/cluster"
)

// removeClassifierReports deletes all ClassifierReport corresponding to Classifier instance
func removeClassifierReports(ctx context.Context, c client.Client, classifier *libsveltosv1alpha1.Classifier,
	logger logr.Logger) error {

	listOptions := []client.ListOption{
		client.MatchingLabels{
			libsveltosv1alpha1.ClassifierLabelName: classifier.Name,
		},
	}

	classifierReportList := &libsveltosv1alpha1.ClassifierReportList{}
	err := c.List(ctx, classifierReportList, listOptions...)
	if err != nil {
		logger.V(logs.LogInfo).Info(fmt.Sprintf("failed to list ClassifierReports. Err: %v", err))
		return err
	}

	for i := range classifierReportList.Items {
		cr := &classifierReportList.Items[i]
		err = c.Delete(ctx, cr)
		if err != nil {
			return err
		}
	}

	return nil
}

// removeClusterClassifierReports deletes all ClassifierReport corresponding to Cluster instance
func removeClusterClassifierReports(ctx context.Context, c client.Client, clusterNamespace, clusterName string,
	logger logr.Logger) error {

	listOptions := []client.ListOption{
		client.MatchingLabels{
			classifierReportClusterLabel: getClusterInfo(clusterNamespace, clusterName),
		},
	}

	classifierReportList := &libsveltosv1alpha1.ClassifierReportList{}
	err := c.List(ctx, classifierReportList, listOptions...)
	if err != nil {
		logger.V(logs.LogInfo).Info(fmt.Sprintf("failed to list ClassifierReports. Err: %v", err))
		return err
	}

	for i := range classifierReportList.Items {
		cr := &classifierReportList.Items[i]
		err = c.Delete(ctx, cr)
		if err != nil {
			return err
		}
	}

	return nil
}

// Periodically collects ClassifierReports from each CAPI cluster.
func collectClassifierReports(c client.Client, logger logr.Logger) {
	const interval = 20 * time.Second

	ctx := context.TODO()
	for {
		clusterList := clusterv1.ClusterList{}
		err := c.List(ctx, &clusterList)
		if err != nil {
			logger.V(logs.LogInfo).Info(fmt.Sprintf("failed to list cluster: %v", err))
			continue
		}
		logger.V(logs.LogDebug).Info("collecting ClassifierReports")

		for i := range clusterList.Items {
			cluster := &clusterList.Items[i]
			err = collectClassifierReportsFromCluster(ctx, c, cluster, logger)
			if err != nil {
				logger.V(logs.LogInfo).Info(fmt.Sprintf("failed to collect ClassifierReports from cluster: %s/%s %v",
					cluster.Namespace, cluster.Name, err))
			}
		}

		time.Sleep(interval)
	}
}

func collectClassifierReportsFromCluster(ctx context.Context, c client.Client,
	cluster *clusterv1.Cluster, logger logr.Logger) error {

	logger = logger.WithValues("cluster", fmt.Sprintf("%s/%s", cluster.Namespace, cluster.Name))
	clusterRef := &corev1.ObjectReference{Namespace: cluster.Namespace, Name: cluster.Name}
	ready, err := clusterproxy.IsClusterReadyToBeConfigured(ctx, c, clusterRef, logger)
	if err != nil {
		logger.V(logs.LogDebug).Info("cluster is not ready yet")
		return err
	}

	if !ready {
		return nil
	}

	scheme, err := InitScheme()
	if err != nil {
		return err
	}

	var remoteClient client.Client
	remoteClient, err = clusterproxy.GetKubernetesClient(ctx, logger, c, scheme, cluster.Namespace, cluster.Name)
	if err != nil {
		return err
	}

	logger.V(logs.LogDebug).Info("collecting ClassifierReports from cluster")
	classifierReportList := libsveltosv1alpha1.ClassifierReportList{}
	err = remoteClient.List(ctx, &classifierReportList)
	if err != nil {
		return err
	}

	for i := range classifierReportList.Items {
		cr := &classifierReportList.Items[i]
		l := logger.WithValues("classifierReport", cr.Name)
		err = updateClassifierReport(ctx, c, cluster, cr, l)
		if err != nil {
			logger.V(logs.LogInfo).Info(fmt.Sprintf("failed to process ClassifierReport. Err: %v", err))
		}
	}

	return nil
}

func updateClassifierReport(ctx context.Context, c client.Client, cluster *clusterv1.Cluster,
	classiferReport *libsveltosv1alpha1.ClassifierReport, logger logr.Logger) error {

	if classiferReport.Labels == nil {
		msg := "classifierReport is malformed. Labels is empty"
		logger.V(logs.LogInfo).Info(msg)
		return errors.New(msg)
	}

	classifierName, ok := classiferReport.Labels[libsveltosv1alpha1.ClassifierLabelName]
	if !ok {
		msg := "classifierReport is malformed. Label missing"
		logger.V(logs.LogInfo).Info(msg)
		return errors.New(msg)
	}

	classifierReportName := getClassifierReportName(classifierName, cluster.Name)

	currentClassifierReport := &libsveltosv1alpha1.ClassifierReport{}
	err := c.Get(ctx,
		types.NamespacedName{Namespace: cluster.Namespace, Name: classifierReportName},
		currentClassifierReport)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.V(logs.LogDebug).Info("create ClassifierReport in management cluster")
			currentClassifierReport.Namespace = cluster.Namespace
			currentClassifierReport.Name = classifierReportName
			currentClassifierReport.Labels = classiferReport.Labels
			currentClassifierReport.Labels[classifierReportClusterLabel] =
				getClusterInfo(cluster.Namespace, cluster.Name)
			currentClassifierReport.Spec = classiferReport.Spec
			currentClassifierReport.Spec.ClusterNamespace = cluster.Namespace
			currentClassifierReport.Spec.ClusterName = cluster.Name
			return c.Create(ctx, currentClassifierReport)
		}
		return err
	}

	logger.V(logs.LogDebug).Info("update ClassifierReport in management cluster")
	currentClassifierReport.Spec = classiferReport.Spec
	currentClassifierReport.Spec.ClusterNamespace = cluster.Namespace
	currentClassifierReport.Spec.ClusterName = cluster.Name
	if currentClassifierReport.Labels == nil {
		currentClassifierReport.Labels = make(map[string]string)
	}
	currentClassifierReport.Labels[classifierReportClusterLabel] =
		getClusterInfo(cluster.Namespace, cluster.Name)
	return c.Update(ctx, currentClassifierReport)
}

func getClassifierReportName(classifierName, clusterName string) string {
	return fmt.Sprintf("%s--%s", classifierName, clusterName)
}

func getClusterInfo(clusterNamespace, clusterName string) string {
	return fmt.Sprintf("%s--%s", clusterNamespace, clusterName)
}
