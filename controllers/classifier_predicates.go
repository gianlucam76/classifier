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
	"github.com/go-logr/logr"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	classifyv1alpha1 "github.com/projectsveltos/classifier/api/v1alpha1"
	logs "github.com/projectsveltos/libsveltos/lib/logsettings"
)

// ClusterPredicates predicates for v1Cluster. ClassifierReconciler watches v1Cluster events
// and react to those by reconciling itself based on following predicates
func ClusterPredicates(logger logr.Logger) predicate.Funcs {
	return predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			newCluster := e.ObjectNew.(*clusterv1.Cluster)
			oldCluster := e.ObjectOld.(*clusterv1.Cluster)
			log := logger.WithValues("predicate", "updateEvent",
				"namespace", newCluster.Namespace,
				"cluster", newCluster.Name,
			)

			if oldCluster == nil {
				log.V(logs.LogVerbose).Info("Old Cluster is nil. Reconcile Classifier")
				return true
			}

			// return true if Cluster.Spec.Paused has changed from true to false
			if oldCluster.Spec.Paused && !newCluster.Spec.Paused {
				log.V(logs.LogVerbose).Info(
					"Cluster was unpaused. Will attempt to reconcile associated Classifiers.")
				return true
			}

			if oldCluster.Status.ControlPlaneReady != newCluster.Status.ControlPlaneReady {
				log.V(logs.LogVerbose).Info(
					"Cluster ControlPlaneReady changed. Will attempt to reconcile associated Classifiers.",
				)
				return true
			}

			if oldCluster.Status.InfrastructureReady != newCluster.Status.InfrastructureReady {
				log.V(logs.LogVerbose).Info(
					"Cluster InfrastructureReady changed. Will attempt to reconcile associated Classifiers.",
				)
				return true
			}

			// otherwise, return false
			log.V(logs.LogVerbose).Info(
				"Cluster did not match expected conditions.  Will not attempt to reconcile associated Classifiers.")
			return false
		},
		CreateFunc: func(e event.CreateEvent) bool {
			cluster := e.Object.(*clusterv1.Cluster)
			log := logger.WithValues("predicate", "createEvent",
				"namespace", cluster.Namespace,
				"cluster", cluster.Name,
			)

			// Only need to trigger a reconcile if the Cluster.Spec.Paused is false
			if !cluster.Spec.Paused {
				log.V(logs.LogVerbose).Info(
					"Cluster is not paused.  Will attempt to reconcile associated Classifiers.",
				)
				return true
			}
			log.V(logs.LogVerbose).Info(
				"Cluster did not match expected conditions.  Will not attempt to reconcile associated Classifiers.")
			return false
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			log := logger.WithValues("predicate", "deleteEvent",
				"namespace", e.Object.GetNamespace(),
				"cluster", e.Object.GetName(),
			)
			log.V(logs.LogVerbose).Info(
				"Cluster deleted.  Will attempt to reconcile associated Classifiers.")
			return true
		},
		GenericFunc: func(e event.GenericEvent) bool {
			log := logger.WithValues("predicate", "genericEvent",
				"namespace", e.Object.GetNamespace(),
				"cluster", e.Object.GetName(),
			)
			log.V(logs.LogVerbose).Info(
				"Cluster did not match expected conditions.  Will not attempt to reconcile associated Classifiers.")
			return false
		},
	}
}

// MachinePredicates predicates for v1Machine. ClassifierReconciler watches v1Machine events
// and react to those by reconciling itself based on following predicates
func MachinePredicates(logger logr.Logger) predicate.Funcs {
	return predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			newMachine := e.ObjectNew.(*clusterv1.Machine)
			oldMachine := e.ObjectOld.(*clusterv1.Machine)
			log := logger.WithValues("predicate", "updateEvent",
				"namespace", newMachine.Namespace,
				"machine", newMachine.Name,
			)

			if newMachine.Status.GetTypedPhase() != clusterv1.MachinePhaseRunning {
				return false
			}

			if oldMachine == nil {
				log.V(logs.LogVerbose).Info("Old Machine is nil. Reconcile Classifier")
				return true
			}

			// return true if Machine.Status.Phase has changed from not running to running
			if oldMachine.Status.GetTypedPhase() != newMachine.Status.GetTypedPhase() {
				log.V(logs.LogVerbose).Info(
					"Machine was not in Running Phase. Will attempt to reconcile associated Classifiers.")
				return true
			}

			// otherwise, return false
			log.V(logs.LogVerbose).Info(
				"Machine did not match expected conditions.  Will not attempt to reconcile associated Classifiers.")
			return false
		},
		CreateFunc: func(e event.CreateEvent) bool {
			machine := e.Object.(*clusterv1.Machine)
			log := logger.WithValues("predicate", "createEvent",
				"namespace", machine.Namespace,
				"machine", machine.Name,
			)

			// Only need to trigger a reconcile if the Machine.Status.Phase is Running
			if machine.Status.GetTypedPhase() == clusterv1.MachinePhaseRunning {
				return true
			}

			log.V(logs.LogVerbose).Info(
				"Machine did not match expected conditions.  Will not attempt to reconcile associated Classifiers.")
			return false
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			log := logger.WithValues("predicate", "deleteEvent",
				"namespace", e.Object.GetNamespace(),
				"machine", e.Object.GetName(),
			)
			log.V(logs.LogVerbose).Info(
				"Machine did not match expected conditions.  Will not attempt to reconcile associated Classifiers.")
			return false
		},
		GenericFunc: func(e event.GenericEvent) bool {
			log := logger.WithValues("predicate", "genericEvent",
				"namespace", e.Object.GetNamespace(),
				"machine", e.Object.GetName(),
			)
			log.V(logs.LogVerbose).Info(
				"Machine did not match expected conditions.  Will not attempt to reconcile associated Classifiers.")
			return false
		},
	}
}

// ClassifierReportPredicate predicates for ClassifierReport. ClassifierReconciler watches ClassifierReport events
// and react to those by reconciling itself based on following predicates
func ClassifierReportPredicate(logger logr.Logger) predicate.Funcs {
	return predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			newReport := e.ObjectNew.(*classifyv1alpha1.ClassifierReport)
			oldReport := e.ObjectOld.(*classifyv1alpha1.ClassifierReport)
			log := logger.WithValues("predicate", "updateEvent",
				"namespace", newReport.Namespace,
				"name", newReport.Name,
			)

			if oldReport == nil {
				log.V(logs.LogVerbose).Info("Old ClassifierReport is nil. Reconcile Classifier")
				return true
			}

			// return true if ClassifierReport.Spec.Match has changed
			if oldReport.Spec.Match != newReport.Spec.Match {
				log.V(logs.LogVerbose).Info(
					"Cluster was unpaused. Will attempt to reconcile associated Classifiers.")
				return true
			}

			// otherwise, return false
			log.V(logs.LogVerbose).Info(
				"ClassifierReport did not match expected conditions.  Will not attempt to reconcile associated Classifiers.")
			return false
		},
		CreateFunc: func(e event.CreateEvent) bool {
			report := e.Object.(*classifyv1alpha1.ClassifierReport)
			log := logger.WithValues("predicate", "createEvent",
				"namespace", report.Namespace,
				"name", report.Name,
			)

			log.V(logs.LogVerbose).Info(
				"Classifier did match expected conditions.  Will attempt to reconcile associated Classifiers.")
			return true
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			report := e.Object.(*classifyv1alpha1.ClassifierReport)
			log := logger.WithValues("predicate", "createEvent",
				"namespace", report.Namespace,
				"name", report.Name,
			)

			log.V(logs.LogVerbose).Info(
				"Classifier did match expected conditions.  Will attempt to reconcile associated Classifiers.")
			return true
		},
		GenericFunc: func(e event.GenericEvent) bool {
			report := e.Object.(*classifyv1alpha1.ClassifierReport)
			log := logger.WithValues("predicate", "createEvent",
				"namespace", report.Namespace,
				"name", report.Name,
			)

			log.V(logs.LogVerbose).Info(
				"Cluster did not match expected conditions.  Will not attempt to reconcile associated Classifiers.")
			return false
		},
	}
}
