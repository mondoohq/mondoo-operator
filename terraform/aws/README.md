# Integration Tests EKS Provisioning

This folder contains Terraform automation code to provision the following:

- **AWS VPC**
- **AWS EKS Cluster** - 2 worker managed nodes (m5.large)

<!-- @import "[TOC]" {cmd="toc" depthFrom=1 depthTo=6 orderedList=false} -->

<!-- code_chunk_output -->

- [Integration Tests EKS Provisioning](#integration-tests-eks-provisioning)
    - [Prerequisites](#prerequisites)
  - [Configuration](#configuration)
    - [Example configuration](#example-configuration)
  - [Provision the cluster](#provision-the-cluster)
  - [Connect to the cluster](#connect-to-the-cluster)
  - [Running integration tests](#running-integration-tests)
  - [Known Issues](#known-issues)
  - [Passing CIS](#passing-cis)

<!-- /code_chunk_output -->

### Prerequisites

- [AWS Account](https://aws.amazon.com/free/)
- [AWS CLI](https://docs.aws.amazon.com/cli/latest/userguide/install-cliv2.html) - `~> aws-cli/2.4.28`
- [Terraform](https://learn.hashicorp.com/tutorials/terraform/install-cli) - `~> v1.0.5`
- [`kubectl`]() - Kubectl must be installed on the host that you run `terraform` from.

## Configuration

Before provisioning set the following environment variables:

- `TF_VAR_region` - AWS region where you want to provision the cluster.
- `TF_VAR_test_name` - This is a prefix that will be applied to all provisioned resources (i.e. `your_name`).

### Example configuration 

Open a terminal and run the following commands:

```bash
export TF_VAR_region=eu-central-1

export TF_VAR_test_name=3d5f6a67b
```

## Provision the cluster

1. Clone the project
```bash title="Clone the project"
git clone git@github.com:mondoohq/mondoo-operator.git
```

2. cd into the terraform folder
```
cd terraform/aws
```

3. Initialize the project (download modules)

```
terraform init
```

4. Check that everything is ready

```
terraform plan
```

5. Apply the configuration

```
terraform apply -auto-approve
```

Once the provisioning completes you will see something like this:

```bash
Apply complete! Resources: 81 added, 0 changed, 0 destroyed.

Outputs:

cluster_arn = "arn:aws:eks:eu-central-1:735798807192:cluster/3d5f6a67b-int-tests-bpke-cluster"
cluster_certificate_authority_data = "..."
cluster_endpoint = "https://FA7565E235EBFB08D1D68D90B420C086.gr7.eu-central-1.eks.amazonaws.com"
cluster_id = "3d5f6a67b-int-tests-bpke-cluster"
cluster_name = "3d5f6a67b-int-tests-bpke-cluster"
cluster_status = "ACTIVE"
region = "eu-central-1"

```

## Connect to the cluster

After Terraform finishes provisioning, set your kube config:
```bash
export KUBECONFIG=./eks-config
```

Then you can connect to your EKS cluster:

```bash
kubectl get nodes
NAME                                       STATUS   ROLES    AGE     VERSION
ip-10-0-5-7.us-east-2.compute.internal     Ready    <none>   6m14s   v1.21.5-eks-9017834
ip-10-0-6-242.us-east-2.compute.internal   Ready    <none>   6m6s    v1.21.5-eks-9017834
```

## Running integration tests

After setting up the cluster via terraform, set your kube config as described above.
Install mondoo-operator via an Integration command.
Now you can run the integration tests of the current branch against EKS by executing:
```bash
EXTERNAL_INSTALLATION=1 K8S_DISTRO=eks VERSION=<latest-tag> make test/integration
```

## Known issues

A refresh of the cluster state might error for the aws_auth config map.
This helps: https://github.com/terraform-aws-modules/terraform-aws-eks/issues/911#issuecomment-726328484

## Passing CIS

To pass the CIS benchmark, you have to delete the default Pod Security Policy:
https://docs.aws.amazon.com/eks/latest/userguide/pod-security-policy.html#psp-delete-default

## License and Author

* Author:: Mondoo Inc

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

## Disclaimer

This or previous program is for Educational purpose ONLY. Do not use it without permission. The usual disclaimer applies, especially the fact that we (Mondoo Inc) is not liable for any damages caused by direct or indirect use of the information or functionality provided by these programs. The author or any Internet provider bears NO responsibility for content or misuse of these programs or any derivatives thereof. By using these programs you accept the fact that any damage (dataloss, system crash, system compromise, etc.) caused by the use of these programs is not Mondoo Inc's responsibility.





