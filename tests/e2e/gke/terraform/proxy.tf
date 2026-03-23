# Copyright Mondoo, Inc. 2026
# SPDX-License-Identifier: BUSL-1.1

################################################################################
# Squid Proxy VM (optional, for registry mirroring / proxy tests)
################################################################################

resource "google_compute_instance" "squid_proxy" {
  count = var.enable_proxy_test ? 1 : 0

  lifecycle {
    precondition {
      condition     = var.enable_mirror_test
      error_message = "enable_proxy_test requires enable_mirror_test to also be true."
    }
  }

  name         = "${local.name_prefix}-squid-proxy"
  project      = var.project_id
  zone         = "${var.region}-a"
  machine_type = "e2-micro"

  boot_disk {
    initialize_params {
      image = "debian-cloud/debian-12"
    }
  }

  network_interface {
    network = "default"
    # No access_config — use IAP tunneling for SSH instead of a public IP
  }

  metadata_startup_script = <<-SCRIPT
#!/bin/bash
set -e
apt-get update
apt-get install -y squid

cat > /etc/squid/squid.conf <<'EOF'
# Mondoo e2e test Squid proxy
acl localnet src 10.0.0.0/8
acl localnet src 172.16.0.0/12
acl localnet src 192.168.0.0/16
acl SSL_ports port 443
acl Safe_ports port 80
acl Safe_ports port 443
acl Safe_ports port 1025-65535
acl CONNECT method CONNECT

http_access allow localnet
http_access allow localhost
http_access deny all

http_port 3128

access_log /var/log/squid/access.log squid
cache_log /var/log/squid/cache.log
EOF

systemctl restart squid
systemctl enable squid
  SCRIPT

  tags = ["squid-proxy"]
}

resource "google_compute_firewall" "allow_squid" {
  count = var.enable_proxy_test ? 1 : 0

  name    = "${local.name_prefix}-allow-squid"
  project = var.project_id
  network = "default"

  allow {
    protocol = "tcp"
    ports    = ["3128"]
  }

  source_ranges = ["10.0.0.0/8"]
  target_tags   = ["squid-proxy"]
}

resource "google_compute_firewall" "allow_squid_iap_ssh" {
  count = var.enable_proxy_test ? 1 : 0

  name    = "${local.name_prefix}-allow-squid-iap-ssh"
  project = var.project_id
  network = "default"

  allow {
    protocol = "tcp"
    ports    = ["22"]
  }

  # IAP's IP range for TCP forwarding
  source_ranges = ["35.235.240.0/20"]
  target_tags   = ["squid-proxy"]
}
