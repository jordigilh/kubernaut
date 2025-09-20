# OpenShift 4.18 Bare Metal Installation with KCLI

This guide provides instructions for deploying OpenShift 4.18 on bare metal infrastructure using KCLI, which significantly simplifies the installation process compared to the traditional openshift-installer approach.

## What is KCLI?

KCLI (Kubernetes CLI) is a tool designed to make it easy to deploy and manage Kubernetes and OpenShift clusters across different platforms. For bare metal deployments, it automates many of the complex tasks involved in cluster provisioning.

## Prerequisites

### Hardware Requirements

#### Provisioner Node (RHEL 9.7 Host)
**Your Host Configuration:**
- **CPU**: Intel Xeon Gold 5218R (20 cores, 40 threads) ✅
- **RAM**: 256 GB ✅
- **Storage**: 3 TB free ✅
- **OS**: RHEL 9.7 with libvirtd ✅

**Resource Allocation for Testing Cluster:**
- **Total VM Usage**: ~84 GB RAM, 24 vCPUs, ~500 GB storage
- **Available Headroom**: 172+ GB RAM, 16+ CPU threads, 2.5+ TB storage
- **Bootstrap Peak**: Additional 16 GB RAM, 4 vCPUs during installation only

#### Virtual Nodes (OpenShift Cluster VMs)
- **Masters**: 3 VMs × (16 GB RAM, 4 vCPUs, 80 GB disk)
- **Workers**: 3 VMs × (12 GB RAM, 4 vCPUs, 80 GB disk)
- **Storage**: Additional virtual disks for ODF testing

### Network Requirements

1. **Baremetal Network**: Production network (e.g., 192.168.122.0/24)
2. **Provisioning Network**: Optional isolated network for PXE (e.g., 172.22.0.0/24)
3. **Internet Access**: Required for image downloads
4. **DNS**: Wildcard DNS or manual DNS entries

## Installation Steps

### Step 1: Install KCLI on RHEL 9.7

#### Recommended Installation for RHEL 9.7

```bash
# Install prerequisites for RHEL 9.7
sudo dnf install -y python3-pip python3-devel libvirt-devel gcc pkg-config libvirt-daemon-kvm qemu-kvm

# Install KCLI
sudo pip3 install kcli[all]

# Enable and start libvirt (should already be running on your host)
sudo systemctl enable --now libvirtd
sudo usermod -aG libvirt $USER

# Reload group membership (or logout/login)
newgrp libvirt

# Verify libvirt is working
virsh list --all
```

#### Option B: Install from PyPI

```bash
# Install dependencies
sudo dnf install -y python3-pip libvirt-devel gcc python3-devel

# Install kcli
pip3 install --user kcli[all]

# Add to PATH
echo 'export PATH=$PATH:$HOME/.local/bin' >> ~/.bashrc
source ~/.bashrc
```

#### Option C: Container Installation

```bash
# Create alias for containerized kcli
alias kcli='podman run --rm -it --security-opt label=disable -v $HOME/.kcli:/root/.kcli -v $HOME/.ssh:/root/.ssh -v /var/lib/libvirt/images:/var/lib/libvirt/images -v /var/run/libvirt:/var/run/libvirt --network host karmab/kcli'
```

### Step 2: Configure KCLI

#### Initialize KCLI configuration

```bash
kcli create host kvm -H 127.0.0.1 local
kcli list host
```

#### Download OpenShift installer and client

```bash
# KCLI can download these automatically, but you can also pre-download
kcli download openshift-install -v 4.18
kcli download oc -v 4.18
```

### Step 3: Prepare Configuration

#### Create a parameter file for your deployment

```bash
mkdir -p ~/.kcli/clusters
```

Create the configuration file `~/.kcli/clusters/ocp418-baremetal-params.yml`:

```yaml
# Cluster Configuration
cluster: ocp418-baremetal
domain: kubernaut.io
version: stable
tag: '4.18'
pull_secret: '/path/to/pull-secret.txt'
ssh_key: '/path/to/ssh-public-key'

# Network Configuration
network: virbr0
network_type: OVNKubernetes
cidr: 192.168.122.0/24
api_ip: 192.168.122.100
ingress_ip: 192.168.122.101

# Provisioning Network (optional)
provisioning_enable: true
provisioning_network: virbr0
provisioning_cidr: 172.22.0.0/24
provisioning_interface: enp1s0
provisioning_ip: 172.22.0.4
provisioning_dhcp_start: 172.22.0.10
provisioning_dhcp_end: 172.22.0.100

# Node Configuration
ctlplane: 3
workers: 3
ctlplane_memory: 32768
ctlplane_numcpus: 8
ctlplane_disk: 120
worker_memory: 16384
worker_numcpus: 4
worker_disk: 120

# Bare Metal Hosts
baremetal_hosts:
  - name: master-0
    mac: "aa:bb:cc:dd:ee:f0"
    ip: "192.168.122.100"
    ipmi_address: "192.168.122.10"
    ipmi_user: "admin"
    ipmi_password: "password"
    role: master
  - name: master-1
    mac: "aa:bb:cc:dd:ee:f1"
    ip: "192.168.122.101"
    ipmi_address: "192.168.122.11"
    ipmi_user: "admin"
    ipmi_password: "password"
    role: master
  - name: master-2
    mac: "aa:bb:cc:dd:ee:f2"
    ip: "192.168.122.102"
    ipmi_address: "192.168.122.12"
    ipmi_user: "admin"
    ipmi_password: "password"
    role: master
  - name: worker-0
    mac: "aa:bb:cc:dd:ee:f3"
    ip: "192.168.122.103"
    ipmi_address: "192.168.122.13"
    ipmi_user: "admin"
    ipmi_password: "password"
    role: worker
  - name: worker-1
    mac: "aa:bb:cc:dd:ee:f4"
    ip: "192.168.122.104"
    ipmi_address: "192.168.122.14"
    ipmi_user: "admin"
    ipmi_password: "password"
    role: worker
  - name: worker-2
    mac: "aa:bb:cc:dd:ee:f5"
    ip: "192.168.122.105"
    ipmi_address: "192.168.122.15"
    ipmi_user: "admin"
    ipmi_password: "password"
    role: worker

# Additional Options
baremetal_iso: true
baremetal_uefi: false
notify: false
apps: []
```

### Step 4: Deploy the Cluster

#### Option A: Deploy with parameter file

```bash
kcli create cluster openshift --paramfile ~/.kcli/clusters/ocp418-baremetal-params.yml ocp418-baremetal
```

#### Option B: Deploy with inline parameters

```bash
kcli create cluster openshift \
  --cluster ocp418-baremetal \
  --domain kubernaut.io \
  --version 4.18 \
  --masters 3 \
  --workers 3 \
  --network baremetal \
  --api-ip 192.168.122.100 \
  --ingress-ip 192.168.122.101 \
  --pull-secret /path/to/pull-secret.txt \
  --ssh-key /path/to/ssh-public-key \
  --baremetal-hosts /path/to/hosts.yaml
```

### Step 5: Monitor Installation

KCLI provides built-in monitoring during installation:

```bash
# Check cluster status
kcli list cluster

# Monitor installation progress
kcli info cluster ocp418-baremetal

# View installation logs
kcli ssh bootstrap -c ocp418-baremetal
sudo journalctl -f -u bootkube.service
```

### Step 6: Access the Cluster

After successful installation:

```bash
# Get kubeconfig
kcli download kubeconfig -c ocp418-baremetal

# Set environment
export KUBECONFIG=$HOME/.kcli/clusters/ocp418-baremetal/auth/kubeconfig

# Verify cluster
oc get nodes
oc get co

# Get console URL and admin password
kcli info cluster ocp418-baremetal | grep console
cat ~/.kcli/clusters/ocp418-baremetal/auth/kubeadmin-password
```

## Advanced KCLI Configuration

### Custom ISO Creation

For environments without PXE boot capability:

```bash
# Create custom ISO with ignition configs
kcli create cluster openshift \
  --paramfile ~/.kcli/clusters/ocp418-baremetal-params.yml \
  --baremetal-iso \
  ocp418-baremetal
```

### Disconnected/Air-gapped Installation

```yaml
# Add to parameter file
disconnected: true
disconnected_url: "registry.kubernaut.io:5000"
disconnected_user: "admin"
disconnected_password: "password"
imagecontentsources: |
  - mirrors:
    - registry.kubernaut.io:5000/ocp4/openshift4
    source: quay.io/openshift-release-dev/ocp-release
  - mirrors:
    - registry.kubernaut.io:5000/ocp4/openshift4
    source: quay.io/openshift-release-dev/ocp-v4.0-art-dev
```

### Single Node OpenShift (SNO)

```bash
kcli create cluster openshift \
  --cluster sno-cluster \
  --sno \
  --api-ip 192.168.122.100 \
  --pull-secret /path/to/pull-secret.txt \
  --ssh-key /path/to/ssh-public-key \
  --network baremetal
```

## KCLI vs OpenShift Installer Comparison

| Feature | KCLI | OpenShift Installer |
|---------|------|-------------------|
| Configuration | Parameter files | install-config.yaml |
| Complexity | Simplified | Complex |
| Automation | High | Medium |
| Customization | Good | Extensive |
| Learning Curve | Easy | Steep |
| Bootstrap Management | Automated | Manual |
| Multi-platform | Yes | Yes |
| Air-gapped Support | Yes | Yes |

## Troubleshooting

### Common Issues

1. **IPMI Connectivity**:
```bash
# Test IPMI access
ipmitool -I lanplus -H 192.168.122.10 -U admin -P password power status
```

2. **Network Connectivity**:
```bash
# Check network bridges
sudo brctl show
sudo ip addr show
```

3. **Disk Space**:
```bash
# Check available space
df -h /var/lib/libvirt/images
```

### Debug Commands

```bash
# KCLI debug mode
kcli -d create cluster openshift --paramfile params.yml cluster-name

# Check cluster logs
kcli console -c cluster-name bootstrap

# SSH to nodes
kcli ssh -c cluster-name master-0
kcli ssh -c cluster-name worker-0
```

### Recovery Operations

```bash
# Stop cluster
kcli stop cluster ocp418-baremetal

# Start cluster
kcli start cluster ocp418-baremetal

# Delete cluster
kcli delete cluster ocp418-baremetal --yes

# Scale workers
kcli scale cluster ocp418-baremetal --workers 5
```

## Storage Configuration

### Automated Storage Setup

KCLI can automatically install and configure Local Storage Operator (LSO) and OpenShift Data Foundation (ODF) for persistent storage. This is configured in the parameter file:

```yaml
# Storage Configuration
storage_operators: true         # Install storage operators automatically
local_storage: true            # Install Local Storage Operator (LSO)
odf: true                      # Install OpenShift Data Foundation (ODF)
odf_size: "2Ti"               # ODF storage size per node
local_storage_devices:         # Local storage devices to use
  - "/dev/sdb"
  - "/dev/sdc"

# Post-deployment Applications and Operators
apps:
  - local-storage-operator      # Local Storage Operator
  - odf-operator               # OpenShift Data Foundation
  - monitoring                 # Enhanced monitoring for storage
```

### Storage Requirements

#### Storage Requirements for Testing Environment
- **Virtual Storage**: KCLI creates additional virtual disks for worker VMs
- **ODF Size**: 200 GB per worker node (total ~600 GB for testing)
- **Storage Backend**: Host filesystem storage (uses your 3TB available space)
- **Performance**: Adequate for testing, uses host I/O capabilities

#### Storage Classes Created
After successful installation, the following storage classes will be available:

1. **local-block**: Raw block storage using Local Storage Operator
2. **local-filesystem**: Filesystem storage using Local Storage Operator
3. **ocs-storagecluster-ceph-rbd**: Ceph RBD storage (default)
4. **ocs-storagecluster-cephfs**: CephFS for shared storage
5. **ocs-storagecluster-ceph-rgw**: Object storage

### Manual Storage Setup

If you prefer manual storage setup or encounter issues with automatic setup:

```bash
# Run the storage setup script manually
chmod +x test/e2e/config/setup-storage.sh
export KUBECONFIG=~/.kcli/clusters/your-cluster/auth/kubeconfig
./test/e2e/config/setup-storage.sh
```

### Storage Configuration Files

The storage setup uses the following configuration files:
- `storage/local-storage-operator.yaml`: LSO configuration
- `storage/odf-operator.yaml`: ODF configuration
- `setup-storage.sh`: Automated setup script

## Post-Installation with KCLI

### Install Additional Components

```bash
# Install operators
kcli create app -c ocp418-baremetal gitops
kcli create app -c ocp418-baremetal pipelines

# Configure authentication
kcli create app -c ocp418-baremetal oauth --paramfile oauth-params.yml
```

### Cluster Management

```bash
# Backup cluster configuration
kcli export cluster ocp418-baremetal

# Update cluster
kcli update cluster ocp418-baremetal --version 4.18.1

# Add nodes
kcli add node -c ocp418-baremetal worker-3 --mac aa:bb:cc:dd:ee:f6 --ipmi 192.168.122.16
```

## Best Practices

1. **Use Parameter Files**: Keep configurations in version control
2. **Test Network Connectivity**: Verify BMC access before deployment
3. **Monitor Resources**: Check CPU, memory, and disk usage during installation
4. **Backup Configurations**: Export cluster configs after successful deployment
5. **Update Regularly**: Keep KCLI updated to the latest version

## Advantages of KCLI Approach

- **Simplified Workflow**: Reduced complexity compared to openshift-installer
- **Automated Bootstrap**: Handles bootstrap node lifecycle automatically
- **Integrated Tools**: Built-in support for monitoring and troubleshooting
- **Multi-Platform**: Same tool works across different environments
- **Active Development**: Regular updates and community support

## Next Steps

After successful installation:

1. Configure authentication providers
2. Set up monitoring and alerting
3. Install required operators
4. Configure networking policies
5. Set up backup and disaster recovery
6. Plan for cluster updates and maintenance

## Resources

- [KCLI Documentation](https://kcli.readthedocs.io/)
- [KCLI GitHub Repository](https://github.com/karmab/kcli)
- [OpenShift Bare Metal Documentation](https://docs.openshift.com/container-platform/4.18/installing/installing_bare_metal_ipi/ipi-install-overview.html)
- [Red Hat OpenShift Documentation](https://docs.openshift.com/)
