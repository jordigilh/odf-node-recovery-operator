package controller

const (
	ODF_NAMESPACE                  = "openshift-storage"
	podStateReasonCrashLoopBackOff = "CrashLoopBackOff"
	FAILED_OSD_IDS                 = "FAILED_OSD_IDS"
	HEALTH_OK                      = "HEALTH_OK"
)

type RecoveryReason string

var (
	EnableCephToolsPod         RecoveryReason = "EnableCephToolsPod"
	WaitForCephToolsPodRunning RecoveryReason = "WaitForCephToolsPodRunning"
	WaitForOSDPodsStabilize    RecoveryReason = "WaitForOSDPodsStabilize"
	LabelNodesWithPendingPods  RecoveryReason = "LabelNodesWithPendingPods"
	ManageCrashLoopBackOffPods RecoveryReason = "ManageCrashLoopBackOffPods"
	RestartStorageOperator     RecoveryReason = "RestartStorageOperator"
	StorageClusterFitnessCheck RecoveryReason = "StorageClusterFitnessCheck"
)
