# Setup of gthm.is

## Provision VM

I provisioned a "learning" VM from Scaleway.
My ISP only gives out IP4 addresses, which apparently means I have to pay for an IP4 address to be able to SSH into the machine.

Add DNS records for `gthm.is` that point to the VM IP.

## Setup VM

Run:

```
$ ansible-playbook -i hosts playbook.yml
```
