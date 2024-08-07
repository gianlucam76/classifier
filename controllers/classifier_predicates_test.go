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

package controllers_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2/textlogger"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/event"

	"github.com/projectsveltos/classifier/controllers"
	libsveltosv1beta1 "github.com/projectsveltos/libsveltos/api/v1beta1"
)

const (
	namespacePrefix = "predicates"
)

var _ = Describe("ClusterProfile Predicates: SvelotsClusterPredicates", func() {
	var logger logr.Logger
	var cluster *libsveltosv1beta1.SveltosCluster

	BeforeEach(func() {
		logger = textlogger.NewLogger(textlogger.NewConfig(textlogger.Verbosity(1)))
		cluster = &libsveltosv1beta1.SveltosCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      upstreamClusterNamePrefix + randomString(),
				Namespace: namespacePrefix + randomString(),
			},
		}
	})

	It("Create reprocesses when sveltos Cluster is unpaused", func() {
		clusterPredicate := controllers.SveltosClusterPredicates(logger)

		cluster.Spec.Paused = false

		e := event.CreateEvent{
			Object: cluster,
		}

		result := clusterPredicate.Create(e)
		Expect(result).To(BeTrue())
	})
	It("Create does not reprocess when sveltos Cluster is paused", func() {
		clusterPredicate := controllers.SveltosClusterPredicates(logger)

		cluster.Spec.Paused = true
		cluster.Annotations = map[string]string{clusterv1.PausedAnnotation: "true"}

		e := event.CreateEvent{
			Object: cluster,
		}

		result := clusterPredicate.Create(e)
		Expect(result).To(BeFalse())
	})
	It("Delete does reprocess ", func() {
		clusterPredicate := controllers.SveltosClusterPredicates(logger)

		e := event.DeleteEvent{
			Object: cluster,
		}

		result := clusterPredicate.Delete(e)
		Expect(result).To(BeTrue())
	})
	It("Update reprocesses when sveltos Cluster paused changes from true to false", func() {
		clusterPredicate := controllers.SveltosClusterPredicates(logger)

		cluster.Spec.Paused = false

		oldCluster := &libsveltosv1beta1.SveltosCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      cluster.Name,
				Namespace: cluster.Namespace,
			},
		}
		oldCluster.Spec.Paused = true
		oldCluster.Annotations = map[string]string{clusterv1.PausedAnnotation: "true"}

		e := event.UpdateEvent{
			ObjectNew: cluster,
			ObjectOld: oldCluster,
		}

		result := clusterPredicate.Update(e)
		Expect(result).To(BeTrue())
	})
	It("Update does not reprocess when sveltos Cluster paused changes from false to true", func() {
		clusterPredicate := controllers.SveltosClusterPredicates(logger)

		cluster.Spec.Paused = true
		cluster.Annotations = map[string]string{clusterv1.PausedAnnotation: "true"}
		oldCluster := &libsveltosv1beta1.SveltosCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      cluster.Name,
				Namespace: cluster.Namespace,
			},
		}
		oldCluster.Spec.Paused = false

		e := event.UpdateEvent{
			ObjectNew: cluster,
			ObjectOld: oldCluster,
		}

		result := clusterPredicate.Update(e)
		Expect(result).To(BeFalse())
	})
	It("Update does not reprocess when sveltos Cluster paused has not changed", func() {
		clusterPredicate := controllers.SveltosClusterPredicates(logger)

		cluster.Spec.Paused = false
		oldCluster := &libsveltosv1beta1.SveltosCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      cluster.Name,
				Namespace: cluster.Namespace,
			},
		}
		oldCluster.Spec.Paused = false

		e := event.UpdateEvent{
			ObjectNew: cluster,
			ObjectOld: oldCluster,
		}

		result := clusterPredicate.Update(e)
		Expect(result).To(BeFalse())
	})
	It("Update reprocesses when sveltos Cluster labels change", func() {
		clusterPredicate := controllers.SveltosClusterPredicates(logger)

		cluster.Labels = map[string]string{"department": "eng"}

		oldCluster := &libsveltosv1beta1.SveltosCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      cluster.Name,
				Namespace: cluster.Namespace,
				Labels:    map[string]string{},
			},
		}

		e := event.UpdateEvent{
			ObjectNew: cluster,
			ObjectOld: oldCluster,
		}

		result := clusterPredicate.Update(e)
		Expect(result).To(BeTrue())
	})
	It("Update reprocesses when sveltos Cluster Status Ready changes", func() {
		clusterPredicate := controllers.SveltosClusterPredicates(logger)

		cluster.Status.Ready = true

		oldCluster := &libsveltosv1beta1.SveltosCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      cluster.Name,
				Namespace: cluster.Namespace,
				Labels:    map[string]string{},
			},
			Status: libsveltosv1beta1.SveltosClusterStatus{
				Ready: false,
			},
		}

		e := event.UpdateEvent{
			ObjectNew: cluster,
			ObjectOld: oldCluster,
		}

		result := clusterPredicate.Update(e)
		Expect(result).To(BeTrue())
	})
})

var _ = Describe("Classifier Predicates: ClusterPredicates", func() {
	var logger logr.Logger
	var cluster *clusterv1.Cluster

	BeforeEach(func() {
		logger = textlogger.NewLogger(textlogger.NewConfig(textlogger.Verbosity(1)))
		cluster = &clusterv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      upstreamClusterNamePrefix + randomString(),
				Namespace: namespacePrefix + randomString(),
			},
		}
	})

	It("Create reprocesses when v1Cluster is unpaused", func() {
		clusterPredicate := controllers.ClusterPredicate{Logger: logger}

		cluster.Spec.Paused = false

		result := clusterPredicate.Create(event.TypedCreateEvent[*clusterv1.Cluster]{Object: cluster})
		Expect(result).To(BeTrue())
	})
	It("Create does not reprocess when v1Cluster is paused", func() {
		clusterPredicate := controllers.ClusterPredicate{Logger: logger}

		cluster.Spec.Paused = true
		cluster.Annotations = map[string]string{clusterv1.PausedAnnotation: "true"}

		result := clusterPredicate.Create(event.TypedCreateEvent[*clusterv1.Cluster]{Object: cluster})
		Expect(result).To(BeFalse())
	})
	It("Delete does reprocess ", func() {
		clusterPredicate := controllers.ClusterPredicate{Logger: logger}

		result := clusterPredicate.Delete(event.TypedDeleteEvent[*clusterv1.Cluster]{Object: cluster})
		Expect(result).To(BeTrue())
	})
	It("Update reprocesses when v1Cluster paused changes from true to false", func() {
		clusterPredicate := controllers.ClusterPredicate{Logger: logger}

		cluster.Spec.Paused = false

		oldCluster := &clusterv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      cluster.Name,
				Namespace: cluster.Namespace,
			},
		}
		oldCluster.Spec.Paused = true
		oldCluster.Annotations = map[string]string{clusterv1.PausedAnnotation: "true"}

		result := clusterPredicate.Update(event.TypedUpdateEvent[*clusterv1.Cluster]{
			ObjectNew: cluster, ObjectOld: oldCluster})
		Expect(result).To(BeTrue())
	})
	It("Update does not reprocess when v1Cluster paused changes from false to true", func() {
		clusterPredicate := controllers.ClusterPredicate{Logger: logger}

		cluster.Spec.Paused = true
		cluster.Annotations = map[string]string{clusterv1.PausedAnnotation: "true"}
		oldCluster := &clusterv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      cluster.Name,
				Namespace: cluster.Namespace,
			},
		}
		oldCluster.Spec.Paused = false

		result := clusterPredicate.Update(event.TypedUpdateEvent[*clusterv1.Cluster]{
			ObjectNew: cluster, ObjectOld: oldCluster})
		Expect(result).To(BeFalse())
	})
	It("Update does not reprocess when v1Cluster paused has not changed", func() {
		clusterPredicate := controllers.ClusterPredicate{Logger: logger}

		cluster.Spec.Paused = false
		oldCluster := &clusterv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      cluster.Name,
				Namespace: cluster.Namespace,
			},
		}
		oldCluster.Spec.Paused = false

		result := clusterPredicate.Update(event.TypedUpdateEvent[*clusterv1.Cluster]{
			ObjectNew: cluster, ObjectOld: oldCluster})
		Expect(result).To(BeFalse())
	})
	It("Update reprocesses when v1Cluster labels change", func() {
		clusterPredicate := controllers.ClusterPredicate{Logger: logger}

		cluster.Labels = map[string]string{"department": "eng"}

		oldCluster := &clusterv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      cluster.Name,
				Namespace: cluster.Namespace,
				Labels:    map[string]string{},
			},
		}

		result := clusterPredicate.Update(event.TypedUpdateEvent[*clusterv1.Cluster]{
			ObjectNew: cluster, ObjectOld: oldCluster})
		Expect(result).To(BeTrue())
	})
})

var _ = Describe("Classifier Predicates: MachinePredicates", func() {
	var logger logr.Logger
	var machine *clusterv1.Machine

	BeforeEach(func() {
		logger = textlogger.NewLogger(textlogger.NewConfig(textlogger.Verbosity(1)))
		machine = &clusterv1.Machine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      upstreamMachineNamePrefix + randomString(),
				Namespace: namespacePrefix + randomString(),
				Labels: map[string]string{
					clusterv1.MachineControlPlaneLabel: "ok",
				},
			},
		}
	})

	It("Create reprocesses when v1Machine is Running", func() {
		machinePredicate := controllers.MachinePredicate{Logger: logger}

		machine.Status.Phase = string(clusterv1.MachinePhaseRunning)

		result := machinePredicate.Create(event.TypedCreateEvent[*clusterv1.Machine]{Object: machine})
		Expect(result).To(BeTrue())
	})
	It("Create does not reprocess when v1Machine is not Running", func() {
		machinePredicate := controllers.MachinePredicate{Logger: logger}

		result := machinePredicate.Create(event.TypedCreateEvent[*clusterv1.Machine]{Object: machine})
		Expect(result).To(BeFalse())
	})
	It("Delete does not reprocess ", func() {
		machinePredicate := controllers.MachinePredicate{Logger: logger}

		result := machinePredicate.Delete(event.TypedDeleteEvent[*clusterv1.Machine]{Object: machine})
		Expect(result).To(BeFalse())
	})
	It("Update reprocesses when v1Machine Phase changed from not running to running", func() {
		machinePredicate := controllers.MachinePredicate{Logger: logger}

		machine.Status.Phase = string(clusterv1.MachinePhaseRunning)

		oldMachine := &clusterv1.Machine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      machine.Name,
				Namespace: machine.Namespace,
			},
		}

		result := machinePredicate.Update(event.TypedUpdateEvent[*clusterv1.Machine]{
			ObjectNew: machine, ObjectOld: oldMachine})
		Expect(result).To(BeTrue())
	})
	It("Update does not reprocess when v1Machine Phase changes from not Phase not set to Phase set but not running", func() {
		machinePredicate := controllers.MachinePredicate{Logger: logger}

		machine.Status.Phase = "Provisioning"

		oldMachine := &clusterv1.Machine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      machine.Name,
				Namespace: machine.Namespace,
			},
		}

		result := machinePredicate.Update(event.TypedUpdateEvent[*clusterv1.Machine]{
			ObjectNew: machine, ObjectOld: oldMachine})
		Expect(result).To(BeFalse())
	})
	It("Update does not reprocess when v1Machine Phases does not change", func() {
		machinePredicate := controllers.MachinePredicate{Logger: logger}
		machine.Status.Phase = string(clusterv1.MachinePhaseRunning)

		oldMachine := &clusterv1.Machine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      machine.Name,
				Namespace: machine.Namespace,
			},
		}
		oldMachine.Status.Phase = machine.Status.Phase

		result := machinePredicate.Update(event.TypedUpdateEvent[*clusterv1.Machine]{
			ObjectNew: machine, ObjectOld: oldMachine})
		Expect(result).To(BeFalse())
	})
})

var _ = Describe("Classifier Predicates: SecretPredicates", func() {
	var logger logr.Logger
	var secret *corev1.Secret

	BeforeEach(func() {
		logger = textlogger.NewLogger(textlogger.NewConfig(textlogger.Verbosity(1)))
		secret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      randomString(),
				Namespace: namespacePrefix + randomString(),
			},
		}
	})

	It("Create reprocesses when label is present", func() {
		secretPredicate := controllers.SecretPredicates(logger)

		secret.Labels = map[string]string{
			libsveltosv1beta1.AccessRequestNameLabel: randomString(),
		}

		e := event.CreateEvent{
			Object: secret,
		}

		result := secretPredicate.Create(e)
		Expect(result).To(BeTrue())
	})

	It("Create does not reprocess when label is not present", func() {
		secretPredicate := controllers.SecretPredicates(logger)

		e := event.CreateEvent{
			Object: secret,
		}

		result := secretPredicate.Create(e)
		Expect(result).To(BeFalse())
	})

	It("Delete does not reprocess ", func() {
		secretPredicate := controllers.SecretPredicates(logger)

		e := event.DeleteEvent{
			Object: secret,
		}

		result := secretPredicate.Delete(e)
		Expect(result).To(BeFalse())
	})

	It("Update reprocesses when Secret Data has changed", func() {
		secretPredicate := controllers.SecretPredicates(logger)

		secret.Data = map[string][]byte{
			randomString(): []byte(randomString()),
		}
		secret.Labels = map[string]string{
			libsveltosv1beta1.AccessRequestNameLabel: randomString(),
		}

		oldSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secret.Name,
				Namespace: secret.Namespace,
				Labels:    secret.Labels,
			},
		}

		e := event.UpdateEvent{
			ObjectNew: secret,
			ObjectOld: oldSecret,
		}

		result := secretPredicate.Update(e)
		Expect(result).To(BeTrue())
	})

	It("Update does not reprocess when Secret Data has not changed", func() {
		secretPredicate := controllers.SecretPredicates(logger)

		secret.Data = map[string][]byte{
			randomString(): []byte(randomString()),
		}
		secret.Labels = map[string]string{
			libsveltosv1beta1.AccessRequestNameLabel: randomString(),
		}

		oldSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secret.Name,
				Namespace: secret.Namespace,
				Labels:    secret.Labels,
			},
			Data: secret.Data,
		}

		e := event.UpdateEvent{
			ObjectNew: secret,
			ObjectOld: oldSecret,
		}

		result := secretPredicate.Update(e)
		Expect(result).To(BeFalse())
	})

	It("Update does not reprocess when Secret has no label", func() {
		secretPredicate := controllers.SecretPredicates(logger)

		secret.Data = map[string][]byte{
			randomString(): []byte(randomString()),
		}

		oldSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secret.Name,
				Namespace: secret.Namespace,
				Labels:    secret.Labels,
			},
		}

		e := event.UpdateEvent{
			ObjectNew: secret,
			ObjectOld: oldSecret,
		}

		result := secretPredicate.Update(e)
		Expect(result).To(BeFalse())
	})
})

var _ = Describe("Classifier Predicates: ConfigMapPredicates", func() {
	var logger logr.Logger
	var configMap *corev1.ConfigMap

	BeforeEach(func() {
		logger = textlogger.NewLogger(textlogger.NewConfig(textlogger.Verbosity(1)))

		configMap = &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "projectsveltos",
				Name:      randomString(),
			},
		}
	})

	AfterEach(func() {
		controllers.SetSveltosAgentConfigMap("")
	})

	It("Create reprocesses when ConfigMap is the one with SveltosAgent configuration", func() {
		name := randomString()
		controllers.SetSveltosAgentConfigMap(name)

		configMapPredicate := controllers.ConfigMapPredicates(logger)

		e := event.CreateEvent{
			Object: configMap,
		}

		result := configMapPredicate.Create(e)
		Expect(result).To(BeFalse())

		configMap.Name = name

		result = configMapPredicate.Create(e)
		Expect(result).To(BeTrue())
	})

	It("Delete reprocesses when ConfigMap is the one with SveltosAgent configuration", func() {
		name := randomString()
		controllers.SetSveltosAgentConfigMap(name)

		configMapPredicate := controllers.ConfigMapPredicates(logger)

		e := event.DeleteEvent{
			Object: configMap,
		}

		result := configMapPredicate.Delete(e)
		Expect(result).To(BeFalse())

		configMap.Name = name

		result = configMapPredicate.Delete(e)
		Expect(result).To(BeTrue())
	})

	It("Update reprocesses when ConfigMap Data has changed", func() {
		name := randomString()
		controllers.SetSveltosAgentConfigMap(name)
		configMap.Name = name

		configMapPredicate := controllers.ConfigMapPredicates(logger)

		configMap.Data = map[string]string{
			randomString(): randomString(),
		}

		oldConfigMap := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      configMap.Name,
				Namespace: configMap.Namespace,
			},
		}

		e := event.UpdateEvent{
			ObjectNew: configMap,
			ObjectOld: oldConfigMap,
		}

		result := configMapPredicate.Update(e)
		Expect(result).To(BeTrue())
	})

	It("Update does not reprocess when ConfigMap Data has not changed", func() {
		name := randomString()
		controllers.SetSveltosAgentConfigMap(name)
		configMap.Name = name

		configMapPredicate := controllers.ConfigMapPredicates(logger)

		configMap.Data = map[string]string{
			randomString(): randomString(),
		}

		oldConfigMap := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      configMap.Name,
				Namespace: configMap.Namespace,
			},
			Data: configMap.Data,
		}

		e := event.UpdateEvent{
			ObjectNew: configMap,
			ObjectOld: oldConfigMap,
		}

		result := configMapPredicate.Update(e)
		Expect(result).To(BeFalse())
	})
})

var _ = Describe("Classifier Predicates: ClassifierReportPredicate", func() {
	var logger logr.Logger
	var report *libsveltosv1beta1.ClassifierReport

	BeforeEach(func() {
		logger = textlogger.NewLogger(textlogger.NewConfig(textlogger.Verbosity(1)))
		report = &libsveltosv1beta1.ClassifierReport{
			ObjectMeta: metav1.ObjectMeta{
				Name:      upstreamClusterNamePrefix + randomString(),
				Namespace: namespacePrefix + randomString(),
			},
		}
	})

	It("Create reprocesses always", func() {
		reportPredicate := controllers.ClassifierReportPredicate(logger)

		e := event.CreateEvent{
			Object: report,
		}

		result := reportPredicate.Create(e)
		Expect(result).To(BeTrue())
	})
	It("Delete does reprocess ", func() {
		reportPredicate := controllers.ClassifierReportPredicate(logger)

		e := event.DeleteEvent{
			Object: report,
		}

		result := reportPredicate.Delete(e)
		Expect(result).To(BeTrue())
	})
	It("Update reprocesses when Spec.Match changes", func() {
		reportPredicate := controllers.ClassifierReportPredicate(logger)

		report.Spec.Match = true

		oldReport := &libsveltosv1beta1.ClassifierReport{
			ObjectMeta: metav1.ObjectMeta{
				Name:      report.Name,
				Namespace: report.Namespace,
			},
		}
		oldReport.Spec.Match = false

		e := event.UpdateEvent{
			ObjectNew: report,
			ObjectOld: oldReport,
		}

		result := reportPredicate.Update(e)
		Expect(result).To(BeTrue())
	})
	It("Update does not reprocess when Spec.Match does not change", func() {
		reportPredicate := controllers.ClassifierReportPredicate(logger)

		report.Spec.Match = true
		report.Labels = map[string]string{randomString(): randomString()}

		oldReport := &libsveltosv1beta1.ClassifierReport{
			ObjectMeta: metav1.ObjectMeta{
				Name:      report.Name,
				Namespace: report.Namespace,
			},
		}
		oldReport.Spec.Match = true

		e := event.UpdateEvent{
			ObjectNew: report,
			ObjectOld: oldReport,
		}

		result := reportPredicate.Update(e)
		Expect(result).To(BeFalse())
	})
})

var _ = Describe("Classifier Predicates: ClassifierPredicate", func() {
	var logger logr.Logger
	var classifier *libsveltosv1beta1.Classifier

	BeforeEach(func() {
		logger = textlogger.NewLogger(textlogger.NewConfig(textlogger.Verbosity(1)))
		classifier = &libsveltosv1beta1.Classifier{
			ObjectMeta: metav1.ObjectMeta{
				Name: randomString(),
			},
		}
	})

	It("Create does not reprocesses", func() {
		classifierPredicate := controllers.ClassifierPredicate(logger)

		e := event.CreateEvent{
			Object: classifier,
		}

		result := classifierPredicate.Create(e)
		Expect(result).To(BeFalse())
	})
	It("Delete does reprocess ", func() {
		classifierPredicate := controllers.ClassifierPredicate(logger)

		e := event.DeleteEvent{
			Object: classifier,
		}

		result := classifierPredicate.Delete(e)
		Expect(result).To(BeTrue())
	})
	It("Update reprocesses when Status.MatchinClusterStatuses changes", func() {
		classifierPredicate := controllers.ClassifierPredicate(logger)

		classifier.Status.MachingClusterStatuses = []libsveltosv1beta1.MachingClusterStatus{
			{
				ClusterRef:    corev1.ObjectReference{Namespace: randomString(), Name: randomString()},
				ManagedLabels: []string{randomString(), randomString()},
			},
		}

		oldClassifier := &libsveltosv1beta1.Classifier{
			ObjectMeta: metav1.ObjectMeta{
				Name: classifier.Name,
			},
		}
		oldClassifier.Status.MachingClusterStatuses = nil

		e := event.UpdateEvent{
			ObjectNew: classifier,
			ObjectOld: oldClassifier,
		}

		result := classifierPredicate.Update(e)
		Expect(result).To(BeTrue())
	})
	It("Update does not reprocess when Status.MatchinClusterStatuses does not change", func() {
		classifierPredicate := controllers.ClassifierPredicate(logger)

		classifier.Status.MachingClusterStatuses = []libsveltosv1beta1.MachingClusterStatus{
			{
				ClusterRef:    corev1.ObjectReference{Namespace: randomString(), Name: randomString()},
				ManagedLabels: []string{randomString(), randomString()},
			},
		}

		oldClassifier := &libsveltosv1beta1.Classifier{
			ObjectMeta: metav1.ObjectMeta{
				Name: classifier.Name,
			},
		}
		oldClassifier.Status.MachingClusterStatuses = classifier.Status.MachingClusterStatuses

		e := event.UpdateEvent{
			ObjectNew: classifier,
			ObjectOld: oldClassifier,
		}

		result := classifierPredicate.Update(e)
		Expect(result).To(BeFalse())
	})
})
