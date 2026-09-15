# Google Cloud IPv6 deployment

This deployment keeps the Shift-My controlled lab on project-owned hostnames and avoids requiring a paid external IPv4 address.

## Recommended free-tier shape

Use an eligible Compute Engine `e2-micro` VM in `us-west1` (Oregon), `us-central1` (Iowa), or `us-east1` (South Carolina), with Ubuntu and no more than 30 GB of Standard Persistent Disk. Oregon is a reasonable default for a US West Coast client.

Networking should use a subnet with external IPv6 enabled. A convenient layout is a dual-stack network interface with its normal internal IPv4 address, **External IPv4 address = None**, and an external IPv6 address. The lab is then publicly reachable only over IPv6.

## Google Cloud console checklist

1. Create or select a Google Cloud project with billing enabled.
2. Create a custom-mode VPC network and a subnet in an eligible free-tier region.
3. Configure the subnet for IPv4 and IPv6 (dual stack) with **External** IPv6 access.
4. Create an Ubuntu `e2-micro` VM attached to that subnet.
5. In the VM network interface, choose **IPv4 and IPv6 (dual-stack)**, set **External IPv4 address** to **None**, and assign an external IPv6 address.
6. Allow inbound TCP/80 and TCP/443 to the VM over IPv6. Keep SSH access restricted to your administration path rather than exposing it broadly.
7. Point an AAAA record for your chosen public hostname at the VM's external IPv6 address. Do not publish an A record unless you intentionally add an external IPv4 address.
8. Confirm the hostname resolves to the VM's IPv6 address before requesting the public certificate.

## Install Shift-My

On the VM, clone this repository and run:

```bash
sudo ./deploy/scripts/bootstrap.sh YOUR_HOSTNAME YOUR_IPV6 YOUR_EMAIL
```

Example:

```bash
sudo ./deploy/scripts/bootstrap.sh lab.example.com 2001:db8::10 you@example.com
```

The example IPv6 address is documentation-only; replace it with the VM's real external IPv6 address.

The bootstrap script stores the address as `SHIFT_MY_PUBLIC_IP`, obtains a public certificate for the hostname, configures Nginx to accept IPv6 connections on TCP/80 and TCP/443, and prints the dashboard credentials.

## Before installing the iPhone profile

The iPhone network must have working IPv6 connectivity to reach an IPv6-only public deployment. Test the dashboard in Safari first. If the dashboard cannot load on the current Wi-Fi network, try an IPv6-capable network or cellular connection before troubleshooting the profile itself.

This guide is for the controlled lab only. It does not route or modify Apple production location services.
