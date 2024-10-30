package controller

import (
	version "github.com/hashicorp/go-version"
)

const (
	ODF_NAMESPACE                  = "openshift-storage"
	podStateReasonCrashLoopBackOff = "CrashLoopBackOff"
	FAILED_OSD_IDS                 = "FAILED_OSD_IDS"
	HEALTH_OK                      = "HEALTH_OK"
	OCS_OSD_REMOVAL                = "ocs-osd-removal"

	podStatusPhaseFieldSelector = ".status.phase"
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
