Coco Elb DNS Registrator 
=================================

[![Circle CI](https://circleci.com/gh/Financial-Times/coco-elb-dns-registrator/tree/master.png?style=shield)](https://circleci.com/gh/Financial-Times/coco-elb-dns-registrator/tree/master)
[![Go Report Card](https://goreportcard.com/badge/github.com/Financial-Times/coco-elb-dns-registrator)](https://goreportcard.com/report/github.com/Financial-Times/coco-elb-dns-registrator)

Registers ELB CNAME in Dyn to the domains passed in as params.

How to Build & Run the binary
-----------------------------

1. Build and test:

        go build
        go test

2. Run:

        export ELB_NAME="coreos-up-176d2040" \
            && export DOMAINS="xp-up,xp-read-up" \
            && export KONSTRUCTOR_BASE_URL="https://dns-api.in.ft.com/v2" \
            && export KONSTRUCTOR_API_KEY="***" \
            && ./coco-elb-dns-registrator