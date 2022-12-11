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
	"fmt"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/projectsveltos/classifier/controllers/keymanager"
	"github.com/projectsveltos/classifier/pkg/scope"
	libsveltosv1alpha1 "github.com/projectsveltos/libsveltos/api/v1alpha1"
	"github.com/projectsveltos/libsveltos/lib/deployer"
	logs "github.com/projectsveltos/libsveltos/lib/logsettings"
	libsveltosset "github.com/projectsveltos/libsveltos/lib/set"
)

type ReportMode int

const (
	// Default mode. In this mode, Classifier running
	// in the management cluster periodically collect
	// ClassifierReport from CAPI clusters
	CollectFromManagementCluster ReportMode = iota

	// In this mode, classifier agent sends ClassifierReport
	// to management cluster.
	// ClassifierAgent is provided with Kubeconfig to access
	// management cluster and can only update ClassifierReport
	AgentSendReportsNoGateway
)

const (
	// deleteRequeueAfter is how long to wait before checking again to see if the cluster still has
	// children during deletion.
	deleteRequeueAfter = 20 * time.Second

	// normalRequeueAfter is how long to wait before checking again to see if the cluster can be moved
	// to ready after or workload features (for instance ingress or reporter) have failed
	normalRequeueAfter = 20 * time.Second

	controlplaneendpoint = "controlplaneendpoint-key"
)

// ClassifierReconciler reconciles a Classifier object
type ClassifierReconciler struct {
	client.Client
	Scheme               *runtime.Scheme
	Deployer             deployer.DeployerInterface
	ConcurrentReconciles int
	ClassifierReportMode ReportMode
	// Management cluster controlplane endpoint. This is needed when mode is AgentSendReportsNoGateway.
	// It will be used by classifier-agent to send classifierreports back to management cluster.
	ControlPlaneEndpoint string
	// use a Mutex to update in-memory structure as MaxConcurrentReconciles is higher than one
	Mux sync.Mutex
	// key: CAPI Cluster namespace/name; value: set of all Classifiers deployed int the Cluster
	// When a Cluster changes, we need to reconcile one or more Classifier (as of now all Classifier
	// are deployed in all Clusters). In order to do so, Classifier reconciler watches for CAPI Cluster
	// changes. Inside a MapFunc there should be no I/O (if that fails, there is no way to recover).
	// So keeps track of Classifier sets deployed in each CAPI Cluster, so that when CAPI Cluster changes
	// list of Classifiers that need reconciliation is in memory.
	// Even though currently each Classifier is deployed in each CAPI Cluster, do not simply keep an in-memory
	// list of all existing Classifier. Rather keep a map per CAPI cluster. If in future, not all Classifiers
	// are deployed in all clusters, map will come in handy.
	// key: CAPI Cluster namespace/name; value: set of all ClusterProfiles matching the Cluster
	ClusterMap map[libsveltosv1alpha1.PolicyRef]*libsveltosset.Set

	// key: Classifier; value: set of CAPI Clusters matched
	ClassifierMap map[libsveltosv1alpha1.PolicyRef]*libsveltosset.Set

	// Contains list of all Classifier with at least one conflict
	ClassifierSet libsveltosset.Set

	// List of current existing Classifiers
	AllClassifierSet libsveltosset.Set
}

//+kubebuilder:rbac:groups=lib.projectsveltos.io,resources=classifiers,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=lib.projectsveltos.io,resources=classifiers/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=lib.projectsveltos.io,resources=classifiers/finalizers,verbs=update
//+kubebuilder:rbac:groups=lib.projectsveltos.io,resources=classifierreports,verbs=get;list;watch;create;update;delete
//+kubebuilder:rbac:groups=lib.projectsveltos.io,resources=accessrequests,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=lib.projectsveltos.io,resources=debuggingconfigurations,verbs=get;list;watch
//+kubebuilder:rbac:groups=cluster.x-k8s.io,resources=clusters,verbs=get;watch;list;update
//+kubebuilder:rbac:groups=cluster.x-k8s.io,resources=clusters/status,verbs=get;watch;list
//+kubebuilder:rbac:groups=cluster.x-k8s.io,resources=machines,verbs=get;watch;list
//+kubebuilder:rbac:groups=cluster.x-k8s.io,resources=machines/status,verbs=get;watch;list
//+kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Classifier object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.12.1/pkg/reconcile
func (r *ClassifierReconciler) Reconcile(ctx context.Context, req ctrl.Request) (_ ctrl.Result, reterr error) {
	logger := ctrl.LoggerFrom(ctx)
	logger.V(logs.LogInfo).Info("Reconciling")

	// Fecth the Classifier instance
	classifier := &libsveltosv1alpha1.Classifier{}
	if err := r.Get(ctx, req.NamespacedName, classifier); err != nil {
		if apierrors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
		logger.Error(err, "Failed to fetch Classifier")
		return reconcile.Result{}, errors.Wrapf(
			err,
			"Failed to fetch Classifier %s",
			req.NamespacedName,
		)
	}

	logger = logger.WithValues("classifier", classifier.Name)

	classifierScope, err := scope.NewClassifierScope(scope.ClassifierScopeParams{
		Client:         r.Client,
		Logger:         logger,
		Classifier:     classifier,
		ControllerName: "classifier",
	})
	if err != nil {
		logger.Error(err, "Failed to create classifierScope")
		return reconcile.Result{}, errors.Wrapf(
			err,
			"unable to create classifier scope for %s",
			req.NamespacedName,
		)
	}

	// Always close the scope when exiting this function so we can persist any Classifier
	// changes.
	defer func() {
		if err := classifierScope.Close(ctx); err != nil {
			reterr = err
		}
	}()

	// Handle deleted classifier
	if !classifier.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, classifierScope)
	}

	// Handle non-deleted classifier
	return r.reconcileNormal(ctx, classifierScope)
}

func (r *ClassifierReconciler) reconcileDelete(
	ctx context.Context,
	classifierScope *scope.ClassifierScope,
) (reconcile.Result, error) {

	logger := classifierScope.Logger
	logger.V(logs.LogInfo).Info("Reconciling Classifier delete")

	err := r.removeAllRegistrations(ctx, classifierScope, logger)
	if err != nil {
		logger.V(logs.LogInfo).Error(err, "failed to clear Classifier label registrations")
		return reconcile.Result{}, err
	}

	r.Mux.Lock()
	defer r.Mux.Unlock()

	classifierInfo := libsveltosv1alpha1.PolicyRef{Kind: libsveltosv1alpha1.ClassifierKind, Name: classifierScope.Name()}
	r.ClassifierSet.Erase(&classifierInfo)
	r.AllClassifierSet.Erase(&classifierInfo)

	f := getHandlersForFeature(libsveltosv1alpha1.FeatureClassifier)
	err = r.undeployClassifier(ctx, classifierScope, f, logger)
	if err != nil {
		logger.V(logs.LogInfo).Error(err, "failed to undeploy")
		return reconcile.Result{Requeue: true, RequeueAfter: deleteRequeueAfter}, nil
	}

	err = removeClassifierReports(ctx, r.Client, classifierScope.Classifier, logger)
	if err != nil {
		logger.V(logs.LogInfo).Error(err, "failed to remove classifierReports")
		return reconcile.Result{Requeue: true, RequeueAfter: deleteRequeueAfter}, nil
	}

	if r.ClassifierReportMode == CollectFromManagementCluster {
		err = removeAccessRequest(ctx, r.Client, logger)
		if err != nil {
			logger.V(logs.LogInfo).Error(err, "failed to remove accessRequest")
			return reconcile.Result{Requeue: true, RequeueAfter: deleteRequeueAfter}, nil
		}
	}

	if controllerutil.ContainsFinalizer(classifierScope.Classifier, libsveltosv1alpha1.ClassifierFinalizer) {
		controllerutil.RemoveFinalizer(classifierScope.Classifier, libsveltosv1alpha1.ClassifierFinalizer)
	}

	logger.V(logs.LogInfo).Info("Reconcile delete success")
	return reconcile.Result{}, nil
}

func (r *ClassifierReconciler) reconcileNormal(
	ctx context.Context,
	classifierScope *scope.ClassifierScope,
) (reconcile.Result, error) {

	logger := classifierScope.Logger
	logger.V(logs.LogInfo).Info("Reconciling Classifier")

	if !controllerutil.ContainsFinalizer(classifierScope.Classifier, libsveltosv1alpha1.ClassifierFinalizer) {
		if err := r.addFinalizer(ctx, classifierScope); err != nil {
			logger.V(logs.LogDebug).Info("failed to update finalizer")
			return reconcile.Result{}, err
		}
	}

	err := r.updateMatchingClustersAndRegistrations(ctx, classifierScope, logger)
	if err != nil {
		logger.V(logs.LogDebug).Info("failed to update matchingClusterRefs")
		return reconcile.Result{}, err
	}

	err = r.updateLabelsOnMatchingClusters(ctx, classifierScope, logger)
	if err != nil {
		logger.V(logs.LogDebug).Info("failed to update cluster labels")
		return reconcile.Result{}, err
	}

	err = r.updateClusterInfo(ctx, classifierScope)
	if err != nil {
		logger.V(logs.LogDebug).Info("failed to update clusterInfo")
		return reconcile.Result{}, err
	}

	r.updateMaps(classifierScope)

	f := getHandlersForFeature(libsveltosv1alpha1.FeatureClassifier)
	if err := r.deployClassifier(ctx, classifierScope, f, logger); err != nil {
		logger.V(logs.LogInfo).Error(err, "failed to deploy")
		return reconcile.Result{Requeue: true, RequeueAfter: normalRequeueAfter}, nil
	}

	logger.V(logs.LogInfo).Info("Reconcile success")
	return reconcile.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ClassifierReconciler) SetupWithManager(mgr ctrl.Manager) error {
	c, err := ctrl.NewControllerManagedBy(mgr).
		For(&libsveltosv1alpha1.Classifier{}).
		WithEventFilter(ifNewDeletedOrSpecChange(mgr.GetLogger())).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: r.ConcurrentReconciles,
		}).
		Build(r)
	if err != nil {
		return errors.Wrap(err, "error creating controller")
	}

	// When classifierReport changes, according to ClassifierReportPredicates,
	// one Classifier needs to be reconciled
	if err := c.Watch(&source.Kind{Type: &libsveltosv1alpha1.ClassifierReport{}},
		handler.EnqueueRequestsFromMapFunc(r.requeueClassifierForClassifierReport),
		ClassifierReportPredicate(mgr.GetLogger().WithValues("predicate", "classifierreportpredicate")),
	); err != nil {
		return err
	}

	// When Classifier changes, according to ClassifierPredicates,
	// all Classifier with at least one conflict needs to be reconciled
	if err := c.Watch(&source.Kind{Type: &libsveltosv1alpha1.Classifier{}},
		handler.EnqueueRequestsFromMapFunc(r.requeueClassifierForClassifier),
		ClassifierPredicate(mgr.GetLogger().WithValues("predicate", "classifiepredicate")),
	); err != nil {
		return err
	}

	// When cluster-api cluster changes, according to ClusterPredicates,
	// one or more Classifiers need to be reconciled.
	if err := c.Watch(&source.Kind{Type: &clusterv1.Cluster{}},
		handler.EnqueueRequestsFromMapFunc(r.requeueClassifierForCluster),
		ClusterPredicates(mgr.GetLogger().WithValues("predicate", "clusterpredicate")),
	); err != nil {
		return err
	}

	// When Secret changes, according to SecretPredicates,
	// Classifiers need to be reconciled.
	if err := c.Watch(&source.Kind{Type: &corev1.Secret{}},
		handler.EnqueueRequestsFromMapFunc(r.requeueClassifierForSecret),
		SecretPredicates(mgr.GetLogger().WithValues("predicate", "secretpredicate")),
	); err != nil {
		return err
	}

	if r.ClassifierReportMode == CollectFromManagementCluster {
		go collectClassifierReports(mgr.GetClient(), mgr.GetLogger())
	}

	// When cluster-api machine changes, according to ClusterPredicates,
	// one or more ClusterProfiles need to be reconciled.
	return c.Watch(&source.Kind{Type: &clusterv1.Machine{}},
		handler.EnqueueRequestsFromMapFunc(r.requeueClassifierForMachine),
		MachinePredicates(mgr.GetLogger().WithValues("predicate", "machinepredicate")),
	)
}

func (r *ClassifierReconciler) getClusterMapForEntry(entry *libsveltosv1alpha1.PolicyRef) *libsveltosset.Set {
	s := r.ClusterMap[*entry]
	if s == nil {
		s = &libsveltosset.Set{}
		r.ClusterMap[*entry] = s
	}
	return s
}

func (r *ClassifierReconciler) addFinalizer(ctx context.Context, classifierScope *scope.ClassifierScope) error {
	// If the SveltosCluster doesn't have our finalizer, add it.
	controllerutil.AddFinalizer(classifierScope.Classifier, libsveltosv1alpha1.ClassifierFinalizer)
	// Register the finalizer immediately to avoid orphaning clusterprofile resources on delete
	if err := classifierScope.PatchObject(ctx); err != nil {
		classifierScope.Error(err, "Failed to add finalizer")
		return errors.Wrapf(
			err,
			"Failed to add finalizer for %s",
			classifierScope.Name(),
		)
	}
	return nil
}

// getListOfClusters returns all CAPI Clusters where Classifier needs to be deployed.
// Currently a Classifier instance needs to be deployed in every existing clusters.
func (r *ClassifierReconciler) getListOfClusters(ctx context.Context, classifierScope *scope.ClassifierScope,
) ([]corev1.ObjectReference, error) {

	clusterList := &clusterv1.ClusterList{}
	if err := r.List(ctx, clusterList); err != nil {
		classifierScope.Logger.Error(err, "failed to list all Cluster")
		return nil, err
	}

	matching := make([]corev1.ObjectReference, 0)

	for i := range clusterList.Items {
		cluster := &clusterList.Items[i]

		if !cluster.DeletionTimestamp.IsZero() {
			// Only existing cluster can match
			continue
		}

		matching = append(matching, corev1.ObjectReference{
			Namespace: cluster.Namespace,
			Name:      cluster.Name,
		})
	}

	return matching, nil
}

// updateClusterInfo updates Classifier Status ClusterInfo by adding an entry for any
// new cluster where Classifier needs to be deployed
func (r *ClassifierReconciler) updateClusterInfo(ctx context.Context, classifierScope *scope.ClassifierScope) error {
	classifier := classifierScope.Classifier

	getClusterID := func(cluster corev1.ObjectReference) string {
		return fmt.Sprintf("%s/%s", cluster.Namespace, cluster.Name)
	}

	matchingCluster, err := r.getListOfClusters(ctx, classifierScope)
	if err != nil {
		return err
	}

	// Build Map for all Clusters with an entry in Classifier.Status.ClusterInfo
	clusterMap := make(map[string]bool)
	for i := range classifier.Status.ClusterInfo {
		c := &classifier.Status.ClusterInfo[i]
		clusterMap[getClusterID(c.Cluster)] = true
	}

	newClusterInfo := make([]libsveltosv1alpha1.ClusterInfo, 0)
	for i := range matchingCluster {
		c := &matchingCluster[i]
		if _, ok := clusterMap[getClusterID(*c)]; !ok {
			newClusterInfo = append(newClusterInfo, libsveltosv1alpha1.ClusterInfo{
				Cluster: *c,
			})
		}
	}

	finalClusterInfo := classifier.Status.ClusterInfo
	finalClusterInfo = append(finalClusterInfo, newClusterInfo...)
	classifierScope.SetClusterInfo(finalClusterInfo)
	return nil
}

// updateMatchingClustersAndRegistrations does two things:
// - updates Classifier Status.MachingClusterStatuses
// - update label key registration with keymanager instance
func (r *ClassifierReconciler) updateMatchingClustersAndRegistrations(ctx context.Context,
	classifierScope *scope.ClassifierScope, logger logr.Logger) error {

	listOptions := []client.ListOption{
		client.MatchingLabels{
			libsveltosv1alpha1.ClassifierLabelName: classifierScope.Classifier.Name,
		},
	}

	classifierReportList := &libsveltosv1alpha1.ClassifierReportList{}
	err := r.List(ctx, classifierReportList, listOptions...)
	if err != nil {
		logger.V(logs.LogInfo).Info(fmt.Sprintf("failed to list ClassifierReports. Err: %v", err))
		return err
	}

	logger.V(logs.LogDebug).Info(fmt.Sprintf("found %d ClassifierReports for this Classifier instance",
		len(classifierReportList.Items)))

	// create map of current matching clusters
	currentMatchingClusters := make(map[corev1.ObjectReference]bool)
	for i := range classifierReportList.Items {
		report := &classifierReportList.Items[i]
		if report.Spec.Match {
			cluster := corev1.ObjectReference{Namespace: report.Spec.ClusterNamespace, Name: report.Spec.ClusterName}
			l := logger.WithValues("cluster", fmt.Sprintf("%s/%s", cluster.Namespace, cluster.Name))
			l.V(logs.LogDebug).Info("is a match")
			currentMatchingClusters[cluster] = true
		}
	}

	// create map of old matching clusters
	oldMatchingClusters := make(map[corev1.ObjectReference]bool)
	for i := range classifierScope.Classifier.Status.MachingClusterStatuses {
		ref := classifierScope.Classifier.Status.MachingClusterStatuses[i]
		oldMatchingClusters[ref.ClusterRef] = true
	}

	err = r.handleLabelRegistrations(ctx, classifierScope.Classifier, currentMatchingClusters,
		oldMatchingClusters, logger)
	if err != nil {
		return err
	}

	matchingClusterStatus := make([]libsveltosv1alpha1.MachingClusterStatus, len(currentMatchingClusters))
	i := 0
	unManaged := 0
	for c := range currentMatchingClusters {
		tmpManaged, tmpUnmanaged, err := r.classifyLabels(ctx, classifierScope.Classifier, &c, logger)
		if err != nil {
			return err
		}
		unManaged += len(tmpUnmanaged)
		matchingClusterStatus[i] =
			libsveltosv1alpha1.MachingClusterStatus{
				ClusterRef:      c,
				ManagedLabels:   tmpManaged,
				UnManagedLabels: tmpUnmanaged,
			}
	}

	r.updateClassifierSet(classifierScope.Name(), unManaged != 0)

	classifierScope.SetMachingClusterStatuses(matchingClusterStatus)

	return nil
}

func (r *ClassifierReconciler) updateClassifierSet(classifierName string, hasUnManaged bool) {
	r.Mux.Lock()
	defer r.Mux.Unlock()

	classifierInfo := libsveltosv1alpha1.PolicyRef{Kind: libsveltosv1alpha1.ClassifierKind, Name: classifierName}
	if hasUnManaged {
		r.ClassifierSet.Insert(&classifierInfo)
	} else {
		r.ClassifierSet.Erase(&classifierInfo)
	}

	r.AllClassifierSet.Insert(&classifierInfo)
}

// updateLabelsOnMatchingClusters set labels on all matching clusters (only for clusters
// for which permission is granted by keymanager)
func (r *ClassifierReconciler) updateLabelsOnMatchingClusters(ctx context.Context,
	classifierScope *scope.ClassifierScope, logger logr.Logger) error {

	// Register Classifier instance as wanting to manage any labels in ClassifierLabels
	// for all the clusters currently matching
	for i := range classifierScope.Classifier.Status.MachingClusterStatuses {
		cluster := &clusterv1.Cluster{}
		ref := classifierScope.Classifier.Status.MachingClusterStatuses[i].ClusterRef
		err := r.Get(ctx, types.NamespacedName{Namespace: ref.Namespace, Name: ref.Name}, cluster)
		if err != nil {
			logger.V(logs.LogInfo).Error(err, fmt.Sprintf("failed to get cluster %s/%s", ref.Namespace, ref.Name))
			return err
		}

		l := logger.WithValues("cluster", fmt.Sprintf("%s/%s", cluster.Namespace, cluster.Name))
		l.V(logs.LogDebug).Info("update labels on cluster")
		err = r.updateLabelsOnCluster(ctx, classifierScope, cluster, l)
		if err != nil {
			l.V(logs.LogDebug).Error(err, "failed to update labels on cluster")
			return err
		}
	}

	return nil
}

func (r *ClassifierReconciler) updateLabelsOnCluster(ctx context.Context,
	classifierScope *scope.ClassifierScope, cluster *clusterv1.Cluster, logger logr.Logger) error {

	manager, err := keymanager.GetKeyManagerInstance(ctx, r.Client)
	if err != nil {
		logger.V(logs.LogInfo).Error(err, "failed to get label key manager")
		return err
	}

	for i := range classifierScope.Classifier.Spec.ClassifierLabels {
		label := classifierScope.Classifier.Spec.ClassifierLabels[i]
		if manager.CanManageLabel(classifierScope.Classifier, cluster.Namespace, cluster.Name, label.Key) {
			if cluster.Labels == nil {
				cluster.Labels = make(map[string]string)
			}
			cluster.Labels[label.Key] = label.Value
		} else {
			l := logger.WithValues("label", label.Key)
			l.V(logs.LogInfo).Info("cannot manage label")
			// Issues is already reported
		}
	}

	return r.Update(ctx, cluster)
}

func (r *ClassifierReconciler) updateMaps(classifierScope *scope.ClassifierScope) {
	currentClusters := &libsveltosset.Set{}
	for i := range classifierScope.Classifier.Status.ClusterInfo {
		cluster := classifierScope.Classifier.Status.ClusterInfo[i].Cluster
		clusterInfo := &libsveltosv1alpha1.PolicyRef{Namespace: cluster.Namespace, Name: cluster.Name, Kind: "Cluster"}
		currentClusters.Insert(clusterInfo)
	}

	r.Mux.Lock()
	defer r.Mux.Unlock()

	classifierInfo := libsveltosv1alpha1.PolicyRef{Kind: libsveltosv1alpha1.ClassifierKind, Name: classifierScope.Name()}

	// Get list of Clusters not matched anymore by Classifier
	var toBeRemoved []libsveltosv1alpha1.PolicyRef
	if v, ok := r.ClassifierMap[classifierInfo]; ok {
		toBeRemoved = v.Difference(currentClusters)
	}

	// For each currently matching Cluster, add Classifier as consumer
	for i := range classifierScope.Classifier.Status.ClusterInfo {
		cluster := classifierScope.Classifier.Status.ClusterInfo[i].Cluster
		clusterInfo := &libsveltosv1alpha1.PolicyRef{Namespace: cluster.Namespace, Name: cluster.Name, Kind: "Cluster"}
		r.getClusterMapForEntry(clusterInfo).Insert(&classifierInfo)
	}

	// For each Cluster not matched anymore, remove Classifier as consumer
	for i := range toBeRemoved {
		clusterName := toBeRemoved[i]
		r.getClusterMapForEntry(&clusterName).Erase(&classifierInfo)
	}

	// Update list of Cluster currently a match for a Classifier
	r.ClassifierMap[classifierInfo] = currentClusters
}

// removeAllRegistrations unregisters Classifier for all cluster labels
// it used to manage (in any matching cluster)
func (r *ClassifierReconciler) removeAllRegistrations(ctx context.Context,
	classifierScope *scope.ClassifierScope, logger logr.Logger,
) error {

	manager, err := keymanager.GetKeyManagerInstance(ctx, r.Client)
	if err != nil {
		logger.V(logs.LogInfo).Error(err, "failed to get label key manager")
		return err
	}

	for i := range classifierScope.Classifier.Status.MachingClusterStatuses {
		c := &classifierScope.Classifier.Status.MachingClusterStatuses[i].ClusterRef
		manager.RemoveAllRegistrations(classifierScope.Classifier, c.Namespace, c.Name)
	}

	return nil
}

// handleLabelRegistrations registers Classifier for all labels, considering all clusters
// currently matching Classifier
// Clear old registrations
func (r *ClassifierReconciler) handleLabelRegistrations(ctx context.Context,
	classifier *libsveltosv1alpha1.Classifier,
	currentMatchingClusters, oldMatchingClusters map[corev1.ObjectReference]bool,
	logger logr.Logger) error {

	// Register Classifier instance as wanting to manage any labels in ClassifierLabels
	// for all the clusters currently matching
	manager, err := keymanager.GetKeyManagerInstance(ctx, r.Client)
	if err != nil {
		logger.V(logs.LogInfo).Error(err, "failed to get label key manager")
		return err
	}

	matchingClusterRefs := make([]corev1.ObjectReference, len(currentMatchingClusters))
	i := 0
	for c := range currentMatchingClusters {
		manager.RemoveStaleRegistrations(classifier, c.Namespace, c.Name)
		manager.RegisterClassifierForLabels(classifier, c.Namespace, c.Name)
		matchingClusterRefs[i] = c
		i++
	}

	// For every cluster which is not a match anymore, remove registations
	for c := range oldMatchingClusters {
		if _, ok := currentMatchingClusters[c]; !ok {
			manager.RemoveAllRegistrations(classifier, c.Namespace, c.Name)
		}
	}

	return nil
}

// classifyLabels divides labels in Managed and UnManaged
func (r *ClassifierReconciler) classifyLabels(ctx context.Context, classifier *libsveltosv1alpha1.Classifier,
	cluster *corev1.ObjectReference, logger logr.Logger) ([]string, []libsveltosv1alpha1.UnManagedLabel, error) {

	manager, err := keymanager.GetKeyManagerInstance(ctx, r.Client)
	if err != nil {
		logger.V(logs.LogInfo).Error(err, "failed to get label key manager")
		return nil, nil, err
	}

	managed := make([]string, 0)
	unManaged := make([]libsveltosv1alpha1.UnManagedLabel, 0)
	for i := range classifier.Spec.ClassifierLabels {
		label := &classifier.Spec.ClassifierLabels[i]
		if manager.CanManageLabel(classifier, cluster.Namespace, cluster.Name, label.Key) {
			logger.V(logs.LogDebug).Info(fmt.Sprintf("classifier can manage label %s", label.Key))
			managed = append(managed, label.Key)
		} else {
			logger.V(logs.LogDebug).Info(fmt.Sprintf("classifier cannot manage label %s", label.Key))
			tmpUnManaged := libsveltosv1alpha1.UnManagedLabel{Key: label.Key}
			currentManager, err := manager.GetManagerForKey(cluster.Namespace, cluster.Name, label.Key)
			if err == nil {
				failureMessage := fmt.Sprintf("classifier %s currently manage this", currentManager)
				tmpUnManaged.FailureMessage = &failureMessage
			}
			unManaged = append(unManaged, tmpUnManaged)
		}
	}

	return managed, unManaged, nil
}
