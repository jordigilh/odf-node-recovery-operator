package monitoring

import (
	"slices"
	"strings"

	"github.com/jordigilh/odf-node-recovery-operator/pkg/api/v1alpha1"
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	TotalOperandInstancesPrometheusCounter = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "odf_node_recovery_total_operand_instances_counter",
		Help: "ODF Node Recovery metric that keeps track of the total number of operand instances created",
	})
	CompletedOperandInstancesPrometheusCounter = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "odf_node_recovery_success_operand_instances_counter",
		Help: "ODF Node Recovery metric that keeps track of the total successful operand instances created",
	}, []string{"osd_ids", "nodes"})
	FailedOperandInstancesPrometheusCounter = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "odf_node_recovery_failed_operand_instances_counter",
		Help: "ODF Node Recovery metric that keeps track of the total number of failed operand instances created, including the last state, the osd number(s) it was attempting to recover, and the hostnames sorted alphabetically",
	}, []string{"osd_ids", "nodes", "last_condition"})

	metricsList = []prometheus.Collector{
		TotalOperandInstancesPrometheusCounter,
		CompletedOperandInstancesPrometheusCounter,
		FailedOperandInstancesPrometheusCounter,
	}
)

func init() {
	for _, metric := range metricsList {
		metrics.Registry.MustRegister(metric)
	}
}

func IncrementFailedOperandCounter(instance *v1alpha1.NodeRecovery, latestCondition *v1alpha1.RecoveryCondition) {
	FailedOperandInstancesPrometheusCounter.With(prometheus.Labels{"osd_ids": strings.Join(instance.Status.CrashedOSDDeploymentIDs, ", "), "nodes": getFailingNodeNames(instance), "last_condition": string(latestCondition.Type)}).Inc()
}

func IncrementCompletedOperandCounter(instance *v1alpha1.NodeRecovery) {
	CompletedOperandInstancesPrometheusCounter.With(prometheus.Labels{"osd_ids": strings.Join(instance.Status.CrashedOSDDeploymentIDs, ", "), "nodes": getFailingNodeNames(instance)}).Inc()
}

func getFailingNodeNames(instance *v1alpha1.NodeRecovery) string {
	n := []string{}
	if len(instance.Status.CrashedOSDDeploymentIDs) > 0 {
		for _, nd := range instance.Status.NodeDevice {
			n = append(n, nd.NodeName)
		}
		slices.Sort(n)
	}
	return strings.Join(n, ", ")
}
