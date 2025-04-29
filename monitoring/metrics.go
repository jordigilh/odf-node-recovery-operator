package monitoring

import (
	"slices"
	"strings"

	"github.com/jordigilh/odf-node-recovery-operator/api/v1alpha1"
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	TotalOperandInstancesPrometheusCounter = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "odf_node_recovery_operand_instances_total",
		Help: "ODF Node Recovery metric that keeps track of the total number of operand instances created",
	})
	CompletedOperandInstancesPrometheusCounter = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "odf_node_recovery_operand_instances_success",
		Help: "ODF Node Recovery metric that keeps track of the total successful operand instances created",
	}, []string{"osd_ids", "nodes"})
	FailedOperandInstancesPrometheusCounter = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "odf_node_recovery_operand_instances_failed",
		Help: "ODF Node Recovery metric that keeps track of the total number of failed operand instances created, including the last state, the osd number(s) it was attempting to recover, and the hostnames sorted alphabetically",
	}, []string{"osd_ids", "nodes", "last_condition"})

	metricsList = []prometheus.Collector{
		TotalOperandInstancesPrometheusCounter,
		CompletedOperandInstancesPrometheusCounter,
		FailedOperandInstancesPrometheusCounter,
	}
)

// RegisterMetrics drops the existing registry and creates a new empty one. Then it proceeds
// to add the metrics for the operator
func RegisterMetrics() {
	// Remove default metrics
	metrics.Registry = prometheus.NewRegistry()
	for _, metric := range metricsList {
		metrics.Registry.MustRegister(metric)
	}
}

// IncrementFailedOperandCounter adds a new metric to the failed operand counter metric
func IncrementFailedOperandCounter(instance *v1alpha1.NodeRecovery, latestCondition *v1alpha1.RecoveryCondition) {
	FailedOperandInstancesPrometheusCounter.With(prometheus.Labels{"osd_ids": strings.Join(instance.Status.CrashedOSDDeploymentIDs, ", "), "nodes": getFailingNodeNames(instance), "last_condition": string(latestCondition.Type)}).Inc()
}

// IncrementCompletedOperandCounter adds a new metric to the completed operand counter
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
