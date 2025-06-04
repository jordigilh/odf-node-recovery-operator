# odf-node-recovery-operator
This operator implements 2 use cases for recovering an ODF cluster in a failing state:
* Recovery via device failure by [replacing a device in a node](https://docs.redhat.com/en/documentation/red_hat_openshift_data_foundation/4.16/html-single/replacing_devices/index#preface-replacing-devices)
* Recovery via node failure by [replacing a node](https://docs.redhat.com/en/documentation/red_hat_openshift_data_foundation/4.14/html-single/replacing_nodes/index#replacing-an-operational-node-using-local-storage-devices_bm-upi-operational)
 with a new one with the same name
## Description
This operator is available in the OpenShift community operators for OCP 4.14-4.18.

## Getting Started

### Prerequisites
- go version v1.23.6+
- podman version 5.4.2+.
- kubectl version v1.11.3+.
- Access to a Kubernetes v1.11.3+ cluster.

### To Deploy on the cluster
**Build and push your image to the location specified by `IMG`:**

```sh
make docker-build docker-push IMG=<some-registry>/odf-node-recovery-operator:tag
```

**NOTE:** This image ought to be published in the personal registry you specified.
And it is required to have access to pull the image from the working environment.
Make sure you have the proper permission to the registry if the above commands don’t work.

**Install the CRDs into the cluster:**

```sh
make install
```

**Deploy the Manager to the cluster with the image specified by `IMG`:**

```sh
make deploy IMG=<some-registry>/odf-node-recovery-operator:tag
```

> **NOTE**: If you encounter RBAC errors, you may need to grant yourself cluster-admin
privileges or be logged in as admin.

**Create instances of your solution**
You can apply the samples (examples) from the config/sample:

```sh
kubectl apply -k config/samples/
```

>**NOTE**: Ensure that the samples has default values to test it out.

### To Uninstall
**Delete the instances (CRs) from the cluster:**

```sh
kubectl delete -k config/samples/
```

**Delete the APIs(CRDs) from the cluster:**

```sh
make uninstall
```

**UnDeploy the controller from the cluster:**

```sh
make undeploy
```

## Metrics

The operator exposes the following prometheus metrics:
* `odf_node_recovery_operand_instances_total_counter` is a counter that tracks the total number of operand instances created
* `odf_node_recovery_completed_operand_instances_counter` is a counter that tracks the total number of operand instances that where successfully applied and recovered the ODF cluster
* `odf_node_recovery_failed_operand_instances_counter` is a counter that tracks the total number of operand instances that failed in their attempt to recover the ODF cluster. It contains a label `last_condition` that exposes the latest recovery state it reached before timing out and failing.
* `odf_node_recovery_success_for_node_counter` is a counter that tracks the total number of successfull attempts in which a node was recovered, with the node label `node` containing the name of the node. The counter does not discern between device or node replacement scenarios.
* `odf_node_recovery_failed_for_node_counter` is a counter that tracks the total numbef of failed attempts in which a node was not recovered, with the node label `node` containing the name of the node. The counter does not discern between device or node replacement scenarios.

## Releasing to community operators
Follow these steps to release a new version in the community-operators repository:
* Update version in Makefile, for instance `v1.1.1`
* Run `make docker-build docker-push` to create and release a new image to the quay.io repository
* Run `make bundle` to generate the `bundle/` files.
* In the [community-operators-prod](https://github.com/redhat-openshift-ecosystem/community-operators-prod.git) repository, create a new directory in `operators/odf-node-recovery-operator/` that matches the new version and copy the contents of the `bundle/` directory in it:

```
$> tree operators/odf-node-recovery-operator
operators/odf-node-recovery-operator
├── 0.0.1
│   ├── manifests
│   │   ├── odf-node-recovery-operator-controller-manager-metrics-service_v1_service.yaml
|   |   |   ....
│   ├── metadata
│   │   └── annotations.yaml
│   └── tests
│       └── scorecard
│           └── config.yaml
├── 0.0.2
│   ├── manifests
│   │   ├── odf-node-recovery-operator-controller-manager-metrics-monitor_monitoring.coreos.com_v1_servicemonitor.yaml
|   |   |   ....
│   ├── metadata
│   │   └── annotations.yaml
│   └── tests
│       └── scorecard
│           └── config.yaml
├── 1.0.0
│   ├── manifests
│   │   ├── odf-node-recovery-operator-controller-manager-metrics-monitor_monitoring.coreos.com_v1_servicemonitor.yaml
|   |   |   ....
│   ├── metadata
│   │   └── annotations.yaml
│   └── tests
│       └── scorecard
│           └── config.yaml
├── 1.1.0
│   ├── manifests
│   │   ├── odf-node-recovery-operator-controller-manager-metrics-monitor_monitoring.coreos.com_v1_servicemonitor.yaml
|   |   |   ....
│   ├── metadata
│   │   └── annotations.yaml
│   └── tests
│       └── scorecard
│           └── config.yaml
```

Create the new directory for 1.1.1:

```
mkdir operators/odf-node-recovery-operator/1.1.1
```
And copy the contents of the `bundle/` directory in this repo to the newly created directory in `community-operators-prod`. You should have
something like this:
```
$> tree operators/odf-node-recovery-operator
operators/odf-node-recovery-operator
├── 0.0.1
|   ....
├── 0.0.2
|   ....
├── 1.0.0
|   ....
├── 1.1.0
|   ....
├── 1.1.1
│   ├── manifests
│   │   ├── odf-node-recovery-operator-controller-manager-metrics-monitor_monitoring.coreos.com_v1_servicemonitor.yaml
|   |   |   ....
│   ├── metadata
│   │   └── annotations.yaml
│   └── tests
│       └── scorecard
│           └── config.yaml
```

* Create a PR in the `community-operators-prod` with the newly created structure. The PR triggers a job that creates the bundle container image and pushes it to a location in `quay.io`. Once completed, it will add an [entry](https://github.com/redhat-openshift-ecosystem/community-operators-prod/pull/6748#issuecomment-2932916521) in the PR with the image pullspec.

* Create a new branch in the `community-operators-prod` to update the catalogs. Update the [operators/odf-node-recovery-operator/catalog-templates/basic.yaml](https://github.com/redhat-openshift-ecosystem/community-operators-prod/blob/main/operators/odf-node-recovery-operator/catalog-templates/basic.yaml) file to include the new bundle image pullspec and also to describe the upgrade path.

* Run the `make catalogs` in `operators/odf-node-recovery-operator/`. The outcome should update all entries in the `catalogs/<OCP version>/odf-node-recovery-operator/catalog.yaml` files to include the new bundle pullspec and the upgrade path as defined in the `basic.yaml` file.

```
$> make catalogs
if [ ! -d /Users/jgil/go/src/github.com/odf/community-operators-prod/bin ]; then mkdir -p /Users/jgil/go/src/github.com/odf/community-operators-prod/bin; fi
curl -sLO https://raw.githubusercontent.com/redhat-openshift-ecosystem/operator-pipelines/main/fbc/render_catalogs.sh
mv render_catalogs.sh /Users/jgil/go/src/github.com/odf/community-operators-prod/bin/render_catalogs.sh
chmod +x /Users/jgil/go/src/github.com/odf/community-operators-prod/bin/render_catalogs.sh
Cleaning up the operator catalogs from /Users/jgil/go/src/github.com/odf/community-operators-prod/catalogs
Rendering catalogs from templates
- Processing template: basic.yaml (Type: olm.template.basic)
 ✅ Rendered → /Users/jgil/go/src/github.com/odf/community-operators-prod/catalogs/v4.12/odf-node-recovery-operator/catalog.yaml
 ✅ Rendered → /Users/jgil/go/src/github.com/odf/community-operators-prod/catalogs/v4.13/odf-node-recovery-operator/catalog.yaml
 ✅ Rendered → /Users/jgil/go/src/github.com/odf/community-operators-prod/catalogs/v4.14/odf-node-recovery-operator/catalog.yaml
 ✅ Rendered → /Users/jgil/go/src/github.com/odf/community-operators-prod/catalogs/v4.15/odf-node-recovery-operator/catalog.yaml
 ✅ Rendered → /Users/jgil/go/src/github.com/odf/community-operators-prod/catalogs/v4.16/odf-node-recovery-operator/catalog.yaml
 ✅ Rendered → /Users/jgil/go/src/github.com/odf/community-operators-prod/catalogs/v4.17/odf-node-recovery-operator/catalog.yaml
 ✅ Rendered → /Users/jgil/go/src/github.com/odf/community-operators-prod/catalogs/v4.18/odf-node-recovery-operator/catalog.yaml
 ```

* Create a PR with the changes. Once merged, the new version should be available within a few hours, depending on when the bespoke OCP cluster updates its catalog.

## Contributing

**NOTE:** Run `make help` for more information on all potential `make` targets

More information can be found via the [Kubebuilder Documentation](https://book.kubebuilder.io/introduction.html)

## License

Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

