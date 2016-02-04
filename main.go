package main

import (
	"flag"
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/elb"
	etcdClient "github.com/coreos/etcd/client"
	etcdContext "golang.org/x/net/context"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
	"io/ioutil"
)

const (
	konsDNSEndPoint = "https://konstructor.ft.com/v1/dns/"
)

var (
	etcdPeers = flag.String("etcdPeers", "", "Comma-separated list of addresses of etcd endpoints to connect to")
	domains = flag.String("domains", "", "Comma-separated list of domains to be registered")
)

type conf struct {
	konsAPIKey      string
	konsDNSEndPoint string
	elbName         string
	awsAccessKey    string
	awsSecretKey    string
	awsRegion       string
}

func set(kapi etcdClient.KeysAPI, s *string, keyName string, e *error) {
	var resp *etcdClient.Response
	if *e != nil {
		return
	}
	resp, *e = kapi.Get(etcdContext.Background(), keyName, nil)
	if *e != nil {
		return
	}
	*s = resp.Node.Value
}

func config() *conf {
	var (
		err error
		c conf
	)

	cfg := etcdClient.Config{
		Endpoints:               strings.Split(*etcdPeers, ","),
		HeaderTimeoutPerRequest: 10 * time.Second,
	}

	etcd, err := etcdClient.New(cfg)
	if err != nil {
		log.Fatal(err)
	}
	kapi := etcdClient.NewKeysAPI(etcd)

	set(kapi, &c.konsAPIKey, "/ft/_credentials/konstructor/api-key", &err)
	set(kapi, &c.elbName, "/ft/_credentials/elb_name", &err)
	set(kapi, &c.awsAccessKey, "/ft/_credentials/aws/aws_access_key_id", &err)
	set(kapi, &c.awsSecretKey, "/ft/_credentials/aws/aws_secret_access_key", &err)
	set(kapi, &c.awsRegion, "/ft/config/aws_region", &err)

	if err != nil {
		log.Fatal(err)
	}

	c.konsDNSEndPoint = konsDNSEndPoint

	return &c
}

func elbDNSName(c *conf) {
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

	c.elbName = *resp.LoadBalancerDescriptions[0].DNSName
}

func destroyDNS(c *conf, domain string, hc *http.Client) error {
	url := fmt.Sprintf("%sdelete?zone=ft.com&name=%s", c.konsDNSEndPoint, domain)
	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		return err
	}
	req.Header.Add("Content-Length", "0")
	req.Header.Add("Accept", "application/json")
	req.Header.Add("Authorization", fmt.Sprintf("Basic %s", c.konsAPIKey))

	response, err := hc.Do(req)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	if response.StatusCode != 200 {
		// if status is not 200, log it, but do not consider it as a service failure
		data, err := ioutil.ReadAll(response.Body)
		message := "Response message could not be obtained"
		if err != nil {
			message = string(data)
		}
		log.Printf("Destroying domain [%v] failed. Response status: [%v], message: [%v]", domain, response.StatusCode, message)
	}
	return nil
}

func createDNS(c *conf, domain string, hc *http.Client) error {
	url := fmt.Sprintf("%screate?zone=ft.com&name=%s&rdata=%s&ttl=600", c.konsDNSEndPoint, domain, c.elbName)
	req, err := http.NewRequest("POST", url, nil)
	if err != nil {
		return err
	}
	req.Header.Add("Content-Length", "0")
	req.Header.Add("Accept", "application/json")
	req.Header.Add("Authorization", fmt.Sprintf("Basic %s", c.konsAPIKey))

	response, err := hc.Do(req)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	if response.StatusCode != 200 {
		// if status is not 200, log it, but do not consider it as a service failure
		data, err := ioutil.ReadAll(response.Body)
		message := "Response message could not be obtained"
		if err != nil {
			message = string(data)
		}
		log.Printf("Creating domain [%v] failed. Response status: [%v], message: [%v]", domain, response.StatusCode, message)
	}
	return nil
}

func main() {
	flag.Parse()
	c := config()
	hc := &http.Client{}

	elbDNSName(c)

	domainsToRegister := strings.Split(*domains, ",")

	for _, domain := range domainsToRegister {
		err := destroyDNS(c, domain, hc)
		if err != nil {
			log.Fatal(err)
		}

		err = createDNS(c, domain, hc)
		if err != nil {
			log.Fatal(err)
		}
	}
}
