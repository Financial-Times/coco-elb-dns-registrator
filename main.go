package main

import (
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/elb"
	"log"
	"os"
)

func main() {

	region := os.Getenv("AWS_REGION")
	if region == "" {
		log.Fatal("missing AWS_REGION environment variable")
	}

	elbName := os.Getenv("ELB_NAME")
	if elbName == "" {
		log.Fatal("missing ELB_NAME environment variable")
	}

	svc := elb.New(
		session.New(
			&aws.Config{
				Region:      aws.String(region),
				Credentials: credentials.NewEnvCredentials(),
			},
		),
	)

	params := &elb.DescribeLoadBalancersInput{
		LoadBalancerNames: []*string{
			aws.String(elbName), // Required
		},
	}

	resp, err := svc.DescribeLoadBalancers(params)

	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("%s", *resp.LoadBalancerDescriptions[0].DNSName)
}
