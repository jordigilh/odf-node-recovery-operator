package controller

import (
	"time"

	version "github.com/hashicorp/go-version"
)

const (
	ODF_NAMESPACE                  = "openshift-storage"
	podStateReasonCrashLoopBackOff = "CrashLoopBackOff"
	podStateReasonError            = "Error"
	FAILED_OSD_IDS                 = "FAILED_OSD_IDS"
	FORCE_OSD_REMOVAL              = "FORCE_OSD_REMOVAL"
	HEALTH_OK                      = "HEALTH_OK"
	OCS_OSD_REMOVAL_JOB            = "ocs-osd-removal"

	reconciliationTimeout = 30 * time.Minute
	osdRemovalJobTimeout  = 10 * time.Minute

	disableForcedOSDRemoval      = false
	enableForcedOSDRemoval       = true
	podStatusPhaseFieldSelector  = ".status.phase"
	pvcStatusPhaseFieldSelector  = ".status.phase"
	podDeletionTimestampSelector = ".metadata.deletionTimestamp"
	osdJobSuccessMessage         = "completed removal"
)

type RecoveryReason string

var (
	EnableCephTools            RecoveryReason = "EnableCephTools"
	WaitForCephToolsPodRunning RecoveryReason = "WaitForCephToolsPodRunning"
	WaitForOSDPodsStabilize    RecoveryReason = "WaitForOSDPodsStabilize"
	LabelNodesWithPendingPods  RecoveryReason = "LabelNodesWithPendingPods"
	ManageCrashLoopBackOffPods RecoveryReason = "ManageCrashLoopBackOffPods"
	RestartStorageOperator     RecoveryReason = "RestartStorageOperator"
	StorageClusterFitnessCheck RecoveryReason = "StorageClusterFitnessCheck"
)

var ocp4_15 *version.Version

func init() {
	var err error
	ocp4_15, err = version.NewVersion("v4.15")
	if err != nil {
		panic("unable to create constant for OCP version 4.15")
	}
}
