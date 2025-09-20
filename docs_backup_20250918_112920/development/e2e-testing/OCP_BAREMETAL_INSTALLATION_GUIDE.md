# OpenShift 4.18 Bare Metal Installation Guide

This guide provides detailed instructions for deploying OpenShift 4.18 on bare metal infrastructure using the openshift-installer CLI and the provided configuration manifest.

## Prerequisites

### Hardware Requirements

#### Control Plane Nodes (3 nodes minimum)
- **CPU**: 4 cores minimum, 8 cores recommended
- **RAM**: 16 GB minimum, 32 GB recommended
- **Storage**: 120 GB minimum for root filesystem
- **Network**: 2 NICs (one for provisioning, one for baremetal network)

#### Worker Nodes (2 nodes minimum, 3 recommended)
- **CPU**: 2 cores minimum, 4 cores recommended
- **RAM**: 8 GB minimum, 16 GB recommended
- **Storage**: 120 GB minimum for root filesystem
- **Network**: 2 NICs (one for provisioning, one for baremetal network)

#### Provisioner Node
- **CPU**: 4 cores minimum
- **RAM**: 8 GB minimum
- **Storage**: 120 GB minimum
- **Network**: Connected to both provisioning and baremetal networks

### Network Configuration

1. **Provisioning Network**: Isolated network for PXE booting and provisioning
   - Example: `172.22.0.0/24`
   - Should not have DHCP enabled (installer will manage DHCP)

2. **Baremetal Network**: Production network for cluster communication
   - Example: `192.168.122.0/24`
   - Must have internet access for image pulls
   - Requires available IP addresses for API and Ingress VIPs

### Software Prerequisites

1. **Red Hat Account**: Required for pull secret and subscription
2. **SSH Key Pair**: For accessing cluster nodes
3. **OpenShift Installer**: Version 4.18 or compatible
4. **BMC Access**: IPMI/Redfish access to all bare metal nodes
5. **DNS Configuration**: Proper DNS records for the cluster

## Required Components

### 1. Pull Secret

Obtain your pull secret from the [Red Hat OpenShift Cluster Manager](https://console.redhat.com/openshift/install/pull-secret).

```bash
# Download your pull secret and save it as pull-secret.txt
# Then encode it for use in the install-config.yaml:
cat pull-secret.txt | jq -c . > formatted-pull-secret.txt
```

### 2. SSH Public Key

Generate an SSH key pair if you don't have one:

```bash
ssh-keygen -t rsa -b 4096 -C "your-email@kubernaut.io"
```

Extract the public key:

```bash
cat ~/.ssh/id_rsa.pub
```

### 3. DNS Records

Configure the following DNS records in your domain:

```
api.ocp418-baremetal.kubernaut.io        A    192.168.122.100
api-int.ocp418-baremetal.kubernaut.io    A    192.168.122.100
*.apps.ocp418-baremetal.kubernaut.io     A    192.168.122.101
```

## Installation Steps

### Step 1: Prepare the Configuration

1. Copy the provided configuration template:
```bash
cp test/e2e/config/install-config-baremetal-4.18.yaml install-config.yaml
```

2. Customize the configuration for your environment:
   - Update `baseDomain` with your actual domain
   - Replace BMC addresses, usernames, and passwords
   - Update MAC addresses for each host
   - Modify IP addresses and network CIDRs as needed
   - Replace the pull secret with your actual pull secret
   - Replace the SSH key with your public key

### Step 2: Hardware-Specific Configuration

For each host in the `platform.baremetal.hosts` section, update:

- **name**: Hostname for the node
- **role**: Either `master` or `worker`
- **bmc.address**: BMC IP address with protocol (ipmi:// or redfish://)
- **bmc.username**: BMC username
- **bmc.password**: BMC password
- **bootMACAddress**: MAC address of the PXE boot interface
- **rootDeviceHints**: Storage device selection criteria

### Step 3: Network Configuration

Update network settings in the configuration:

- **apiVIPs**: Virtual IP for API server
- **ingressVIPs**: Virtual IP for ingress/router
- **machineNetwork**: CIDR for the baremetal network
- **provisioningNetworkCIDR**: CIDR for provisioning network
- **provisioningDHCPRange**: DHCP range for provisioning

### Step 4: Install OpenShift

1. Create the installation directory:
```bash
mkdir ocp418-baremetal-install
cp install-config.yaml ocp418-baremetal-install/
```

2. Run the installer:
```bash
./openshift-install create cluster --dir ocp418-baremetal-install --log-level=debug
```

The installation process will:
- Validate the configuration
- Create ignition files
- Boot the bootstrap node
- Provision control plane nodes
- Provision worker nodes
- Complete cluster initialization

### Step 5: Access the Cluster

Once installation completes:

1. Set up kubeconfig:
```bash
export KUBECONFIG=ocp418-baremetal-install/auth/kubeconfig
```

2. Verify cluster status:
```bash
oc get nodes
oc get co  # Check cluster operators
```

3. Get console URL:
```bash
oc get routes console -n openshift-console
```

4. Get admin credentials:
```bash
cat ocp418-baremetal-install/auth/kubeadmin-password
```

## Configuration Parameters Explanation

### Essential Parameters

- **baseDomain**: Your organization's domain name
- **metadata.name**: Cluster name (will become part of FQDN)
- **pullSecret**: Authentication token for Red Hat registries
- **sshKey**: SSH public key for node access

### Networking Parameters

- **clusterNetwork.cidr**: Pod network CIDR
- **serviceNetwork**: Service network CIDR
- **machineNetwork.cidr**: Host network CIDR
- **apiVIPs/ingressVIPs**: Load balancer virtual IPs

### Bare Metal Specific Parameters

- **provisioningNetworkInterface**: NIC name for provisioning network
- **provisioningNetworkCIDR**: Provisioning network CIDR
- **provisioningDHCPRange**: DHCP range for PXE boot
- **hosts**: Detailed hardware inventory

## Troubleshooting

### Common Issues

1. **BMC Connectivity**: Ensure BMC interfaces are reachable from provisioner
2. **MAC Address Mismatch**: Verify MAC addresses match PXE boot interfaces
3. **Network Isolation**: Ensure provisioning network is properly isolated
4. **Power Management**: Verify BMC credentials and power control capabilities
5. **Storage Detection**: Ensure root device hints match available storage

### Debug Commands

```bash
# Check installer logs
tail -f ocp418-baremetal-install/.openshift_install.log

# Monitor bootstrap progress
ssh -i ~/.ssh/id_rsa core@bootstrap-ip
sudo journalctl -f -u bootkube.service

# Check cluster operators
oc get co
oc describe co <operator-name>

# Check node status
oc get nodes -o wide
oc describe node <node-name>
```

### Recovery Options

If installation fails:

1. **Clean up and retry**:
```bash
./openshift-install destroy cluster --dir ocp418-baremetal-install
# Fix configuration issues
./openshift-install create cluster --dir ocp418-baremetal-install
```

2. **Gather logs**:
```bash
./openshift-install gather bootstrap --dir ocp418-baremetal-install
```

## Post-Installation Tasks

1. **Configure authentication** (LDAP, OIDC, etc.)
2. **Set up monitoring and logging**
3. **Configure networking policies**
4. **Install operators from OperatorHub**
5. **Configure backup solutions**
6. **Set up persistent storage**

## Important Notes

- Keep the `install-config.yaml` file backup as it gets consumed during installation
- The bootstrap node will be automatically destroyed after successful installation
- Ensure BMC time synchronization for proper certificate validation
- Consider using a bastion host for secure access to the provisioning network
- Plan for certificate rotation and cluster updates

## Support Resources

- [OpenShift Documentation](https://docs.openshift.com/container-platform/4.18/)
- [Red Hat Customer Portal](https://access.redhat.com/)
- [OpenShift Community](https://community.openshift.com/)
- [Red Hat Training](https://www.redhat.com/en/services/training)
