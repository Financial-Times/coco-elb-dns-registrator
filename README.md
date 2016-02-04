#coco-elb-dns-registrator

Registers ELB CNAME in Dyn.

Environment parameters to be passed:

* ETCD_PEERS - Comma-separated list of addresses of etcd endpoints to connect to
* DOMAINS - Comma-separated list of domains to be registered
