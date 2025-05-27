package monitoring

import (
	"github.com/jordigilh/odf-node-recovery-operator/api/v1alpha1"
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	// Operand
	TotalOperandInstancesPrometheusCounter = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "odf_node_recovery_operand_instances_total_counter",
		Help: "ODF Node Recovery metric that keeps track of the total number of operand instances created",
	})
	CompletedOperandInstancesPrometheusCounter = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "odf_node_recovery_completed_operand_instances_counter",
		Help: "ODF Node Recovery metric is a counter that tracks the total number of operand instances that completed successfully",
	})
	FailedOperandInstancesPrometheusCounter = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "odf_node_recovery_failed_operand_instances_counter",
		Help: "ODF Node Recovery metric is a counter that tracks the total number of operands that failed to recover the ODF cluster and the last condition reached",
	}, []string{"last_condition"})

	// Node
	SuccessRecoveryForNodePrometheusCounter = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "odf_node_recovery_success_for_node_counter",
		Help: "ODF Node Recovery metric is a counter that tracks the total successfully attempts to recover a given node",
	}, []string{"node"})
	FailedRecoveryForNodePrometheusCounter = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "odf_node_recovery_failed_for_node_counter",
		Help: "ODF Node Recovery metric is a counter that tracks of the number of failed attemps to recover a given node",
	}, []string{"node"})

	metricsList = []prometheus.Collector{
		// Operand
		TotalOperandInstancesPrometheusCounter,
		CompletedOperandInstancesPrometheusCounter,
		FailedOperandInstancesPrometheusCounter,
		// Node
		SuccessRecoveryForNodePrometheusCounter,
		FailedRecoveryForNodePrometheusCounter,
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

// IncrementFailedNodeCounter adds a new metric to the failed node recovery counter metric
func IncrementFailedNodeCounter(instance *v1alpha1.NodeRecovery, latestCondition *v1alpha1.RecoveryCondition) {
	for _, nd := range instance.Status.NodeDevice {
		FailedRecoveryForNodePrometheusCounter.With(prometheus.Labels{"node": nd.NodeName}).Inc()
	}
}

// IncrementCompletedOperandCounter adds a new metric to the completed node recovery counter metric
func IncrementCompletedOperandCounter(instance *v1alpha1.NodeRecovery) {
	for _, nd := range instance.Status.NodeDevice {
		SuccessRecoveryForNodePrometheusCounter.With(prometheus.Labels{"node": nd.NodeName}).Inc()
	}
}
