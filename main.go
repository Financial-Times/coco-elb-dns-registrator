package main

import (
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/elb"
	"github.com/jawher/mow.cli"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"time"
)

func main() {
	app := cli.App("elb dns registrator", "Registers elb cname to *-up.ft.com cnames in dyn using konstructor")

	domains := app.String(cli.StringOpt{
		Name:   "domains",
		Desc:   "comma separated *-up domains",
		EnvVar: "DOMAINS",
	})
	konstructorBaseUrl := app.String(cli.StringOpt{
		Name:   "konstructor-base-url",
		Desc:   "konstructor base url: https://dns-api.in.ft.com/v2",
		EnvVar: "KONSTRUCTOR_BASE_URL",
		Value:  "https://dns-api.in.ft.com/v2",
	})
	konstructorApiKey := app.String(cli.StringOpt{
		Name:   "konstructor-api-key",
		Desc:   "konstructor api key",
		EnvVar: "KONSTRUCTOR_API_KEY",
	})
	elbName := app.String(cli.StringOpt{
		Name:   "elb-name",
		Desc:   "elb cname",
		EnvVar: "ELB_NAME",
	})
	awsRegion := app.String(cli.StringOpt{
		Name:   "aws-region",
		Desc:   "aws region",
		EnvVar: "AWS_REGION",
	})
	awsAccessKeyID := app.String(cli.StringOpt{
		Name:   "aws_access_key_id",
		Desc:   "aws access key id",
		EnvVar: "AWS_ACCESS_KEY_ID",
	})
	awsSecretAccessKey := app.String(cli.StringOpt{
		Name:   "aws_secret_access_key",
		Desc:   "aws secret access key",
		EnvVar: "AWS_SECRET_ACCESS_KEY",
	})

	app.Action = func() {
		c := &conf{
			konsAPIKey:      *konstructorApiKey,
			konsDNSEndPoint: *konstructorBaseUrl,
			elbName:         *elbName,
			awsAccessKey:    *awsAccessKeyID,
			awsSecretKey:    *awsSecretAccessKey,
			awsRegion:       *awsRegion,
		}
		httpClient := &http.Client{
			Transport: &http.Transport{
				MaxIdleConnsPerHost: 128,
				Dial: (&net.Dialer{
					Timeout:   30 * time.Second,
					KeepAlive: 30 * time.Second,
				}).Dial,
			},
		}

		elbCNAME := elbDNSName(c)
		domainsToRegister := strings.Split(*domains, ",")

		for _, domain := range domainsToRegister {
			err := destroyDNS(c, domain, httpClient)
			if err != nil {
				log.Fatal(err)
			}

			err = createDNS(c, elbCNAME, domain, httpClient)
			if err != nil {
				log.Fatal(err)
			}
		}
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Fatalf("[%v]", err)
	}
}

type conf struct {
	konsAPIKey      string
	konsDNSEndPoint string
	elbName         string
	awsAccessKey    string
	awsSecretKey    string
	awsRegion       string
}

func elbDNSName(c *conf) string {
	//weirdness around how aws handles credentials
	os.Setenv("AWS_ACCESS_KEY_ID", c.awsAccessKey)
	os.Setenv("AWS_SECRET_ACCESS_KEY", c.awsSecretKey)

	svc := elb.New(
		session.New(
			&aws.Config{
				Region:      aws.String(c.awsRegion),
				Credentials: credentials.NewEnvCredentials(),
			},
		),
	)

	params := &elb.DescribeLoadBalancersInput{
		LoadBalancerNames: []*string{
			aws.String(c.elbName), // Required
		},
	}

	resp, err := svc.DescribeLoadBalancers(params)

	if err != nil {
		log.Fatal(err)
	}

	return *resp.LoadBalancerDescriptions[0].DNSName
}

func destroyDNS(c *conf, domain string, hc *http.Client) error {
	body := fmt.Sprintf("{\"zone\": \"ft.com\", \"name\": \"%s\"}", domain)
	req, err := http.NewRequest("DELETE", c.konsDNSEndPoint, strings.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Accept", "application/json")
	req.Header.Add("x-api-key", c.konsAPIKey)

	response, err := hc.Do(req)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	if response.StatusCode != 200 {
		// if status is not 200, log it, but do not consider it as a service failure
		data, err := ioutil.ReadAll(response.Body)
		message := "Response message could not be obtained"
		if err == nil {
			message = string(data)
		}
		log.Printf("Destroying domain [%v] failed. Response status: [%v], message: [%v]", domain, response.StatusCode, message)
	}
	return nil
}

func createDNS(c *conf, elbCNAME string, domain string, hc *http.Client) error {
	body := fmt.Sprintf("{\"zone\": \"ft.com\", \"name\": \"%s\",\"rdata\": \"%s\",\"ttl\": \"600\"}", domain, elbCNAME)
	req, err := http.NewRequest("POST", c.konsDNSEndPoint, strings.NewReader(body))
	if err != nil {
		return err
	}

	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Accept", "application/json")
	req.Header.Add("x-api-key", c.konsAPIKey)

	response, err := hc.Do(req)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	if response.StatusCode != 200 {
		// if status is not 200, log it, but do not consider it as a service failure
		data, err := ioutil.ReadAll(response.Body)
		message := "Response message could not be obtained"
		if err == nil {
			message = string(data)
		}
		log.Printf("Creating domain [%v] failed. Response status: [%v], message: [%v]", domain, response.StatusCode, message)
	}
	return nil
}
