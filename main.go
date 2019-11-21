package main

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	cli "github.com/jawher/mow.cli"
	"k8s.io/client-go/1.5/kubernetes"
	"k8s.io/client-go/1.5/rest"
)

var httpClient = http.Client{
	Transport: &http.Transport{
		MaxIdleConnsPerHost: 128,
		Dial: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).Dial,
	},
}

func main() {
	app := cli.App("elb dns registrator", "Registers elb cname to *-up.ft.com cnames in dyn using konstructor")

	domainZone := app.String(cli.StringOpt{
		Name:   "domainZone",
		Desc:   "*-up domain zone",
		EnvVar: "DomainZone",
		Value:  "ft.com",
	})
	domains := app.String(cli.StringOpt{
		Name:   "domains",
		Desc:   "comma separated *-up domains",
		EnvVar: "DOMAINS",
	})
	konstructorBaseURL := app.String(cli.StringOpt{
		Name:   "konstructor-base-url",
		Desc:   "konstructor base url: https://dns-api.in.ft.com/v2",
		EnvVar: "KONSTRUCTOR_BASE_URL",
		Value:  "https://dns-api.in.ft.com/v2",
	})
	konstructorAPIKey := app.String(cli.StringOpt{
		Name:   "konstructor-api-key",
		Desc:   "konstructor api key",
		EnvVar: "KONSTRUCTOR_API_KEY",
	})

	kubeLbService := app.String(cli.StringOpt{
		Name:   "k8s-lb-service",
		Desc:   "The Kubernetes service of type 'LoadBalancer' that we should register the ELB for",
		EnvVar: "K8S_LB_SERVICE",
	})

	kubeLBServiceNamespace := app.String(cli.StringOpt{
		Name:   "k8s-lb-service-namespace",
		Desc:   "The Kubernetes namespace of the service of type 'LoadBalancer' that we should register the ELB for",
		EnvVar: "K8S_LB_SERVICE_NAMESPACE",
	})

	app.Action = func() {
		conf := &conf{
			konsAPIKey:             *konstructorAPIKey,
			konsDNSEndPoint:        *konstructorBaseURL,
			kubeLbService:          *kubeLbService,
			kubeLbServiceNamespace: *kubeLBServiceNamespace,
		}

		elbCNAME := getKubeElbDNSCname(conf)
		domainsToRegister := strings.Split(*domains, ",")

		for _, domain := range domainsToRegister {
			currentCNAME, err := getCurrentCNAME(conf, *domainZone, domain)
			if err != nil {
				log.Fatalf("ERROR - [%v]", err)
			}
			if currentCNAME == "" {
				err = createDNS(conf, elbCNAME, domain, *domainZone)
			} else {
				err = updateDNS(conf, currentCNAME, elbCNAME, domain, *domainZone)
			}
			if err != nil {
				log.Fatalf("ERROR - [%v]", err)
			}
		}
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Fatalf("ERROR - [%v]", err)
	}
}

func getKubeElbDNSCname(conf *conf) string {
	// creates the in-cluster config
	config, err := rest.InClusterConfig()
	if err != nil {
		log.Fatalf("ERROR - Could not get the K8s cluster config: [%v]", err)
	}
	// creates the clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Fatalf("ERROR - Could not get the client for K8s: [%v]", err)
	}

	lbService, err := clientset.Core().Services(conf.kubeLbServiceNamespace).Get(conf.kubeLbService)
	if err != nil {
		log.Fatalf("ERROR - Could not get the K8S LB service '%s'. Reason: [%v]", conf.kubeLbService, err)
	}

	if len(lbService.Status.LoadBalancer.Ingress) == 0 {
		log.Fatalf("ERROR - No ingress address found for LB service '%s'", conf.kubeLbService)
	}

	return lbService.Status.LoadBalancer.Ingress[0].Hostname
}

type conf struct {
	konsAPIKey             string
	konsDNSEndPoint        string
	elbName                string
	kubeLbService          string
	kubeLbServiceNamespace string
}

func getCurrentCNAME(c *conf, domainZone, domain string) (string, error) {
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/name/%s/%s", c.konsDNSEndPoint, domainZone, domain), nil)
	if err != nil {
		return "", err
	}
	req.Header.Add("Accept", "application/json")
	req.Header.Add("x-api-key", c.konsAPIKey)

	response, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("Could not connect to Konstructor, [%v]", err)
	}
	defer func() {
		io.Copy(ioutil.Discard, response.Body)
		response.Body.Close()
	}()

	data, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return "", fmt.Errorf("Could not read konstructor response body, statusCode=[%v], [%v]", response.StatusCode, err)
	}
	if response.StatusCode != http.StatusOK {
		// if status is not 200, log it, it means domain does not exist
		log.Printf("INFO - Domain=[%v] not created, statusCode=[%v], message=[%v]", domain, response.StatusCode, string(data))
		return "", nil
	}

	type konstructorRes struct {
		CNAMES []string `json:"records"`
	}
	r := konstructorRes{}
	if err := json.Unmarshal(data, &r); err != nil {
		return "", err
	}

	//remove trailing "." otherwise update method complains
	return strings.TrimSuffix(r.CNAMES[0], "."), nil
}

func createDNS(c *conf, elbCNAME string, domain string, domainZone string) error {
	body := fmt.Sprintf("{\"zone\": \"%s\", \"name\": \"%s\",\"rdata\": \"%s\",\"ttl\": \"30\",\"emailAddress\": \"universal.publishing.platform@ft.com\"}", domainZone, domain, elbCNAME)
	req, err := http.NewRequest(http.MethodPost, c.konsDNSEndPoint, strings.NewReader(body))
	if err != nil {
		return err
	}
	if err = executeReq(req, c.konsAPIKey); err != nil {
		return fmt.Errorf("Creating domain=[%v] failed, %v", domain, err)
	}
	return nil
}

func updateDNS(c *conf, oldCname string, newCname, domain string, domainZone string) error {
	body := fmt.Sprintf("{\"zone\": \"%s\", \"name\": \"%s\",\"oldRdata\": \"%s\",\"newRdata\": \"%s\",\"ttl\": \"30\",\"emailAddress\": \"universal.publishing.platform@ft.com\"}", domainZone, domain, oldCname, newCname)
	req, err := http.NewRequest(http.MethodPut, c.konsDNSEndPoint, strings.NewReader(body))
	if err != nil {
		return err
	}
	if err = executeReq(req, c.konsAPIKey); err != nil {
		return fmt.Errorf("Updating domain=[%v] failed, %v", domain, err)
	}
	return nil
}

func executeReq(req *http.Request, key string) error {
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Accept", "application/json")
	req.Header.Add("x-api-key", key)

	response, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("Could not connect to Konstructor, [%v]", err)
	}
	defer func() {
		io.Copy(ioutil.Discard, response.Body)
		response.Body.Close()
	}()

	if response.StatusCode != http.StatusOK {
		// if status is not 200, log it, but do not consider it as a service failure
		data, err := ioutil.ReadAll(response.Body)
		message := "Response message could not be obtained"
		if err == nil {
			message = string(data)
		}
		return fmt.Errorf("statusCode=[%v], message=[%v]", response.StatusCode, message)
	}
	return nil
}
