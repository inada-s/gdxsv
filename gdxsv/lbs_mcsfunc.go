package main

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/golang/glog"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

var (
	mcsFuncClientCreated time.Time
	mcsFuncClientCache   *http.Client
	mcsFuncRequestTime   = map[string]time.Time{}
)

var gcpLocationName = map[string]string{
	"asia-east1":              "Changhua County, Taiwan",
	"asia-east2":              "Hong Kong",
	"asia-northeast1":         "Tokyo, Japan",
	"asia-northeast2":         "Osaka, Japan",
	"asia-northeast3":         "Seoul, South Korea",
	"asia-south1":             "Mumbai, India",
	"asia-southeast1":         "Jurong West, Singapore",
	"australia-southeast1":    "Sydney, Australia",
	"europe-north1":           "Hamina, Finland",
	"europe-west1":            "St. Ghislain, Belgium",
	"europe-west2":            "London, England, UK",
	"europe-west3":            "Frankfurt, Germany",
	"europe-west4":            "Eemshaven, Netherlands",
	"europe-west6":            "Zürich, Switzerland",
	"northamerica-northeast1": "Montreal, Quebec, Canada",
	"southamerica-east1":      "Osasco (Sao Paulo), Brazil",
	"us-central1":             "Council Bluffs, Iowa, USA",
	"us-east1":                "Moncks Corner, South Carolina, USA",
	"us-east4":                "Ashburn, Northern Virginia, USA",
	"us-west1":                "The Dalles, Oregon, USA",
	"us-west2":                "Los Angeles, California, USA",
	"us-west3":                "Salt Lake City, Utah, USA",
}

func getMcsFuncClient() (*http.Client, error) {
	if mcsFuncClientCache != nil && time.Since(mcsFuncClientCreated).Minutes() <= 30.0 {
		return mcsFuncClientCache, nil
	}

	jsonKey, err := ioutil.ReadFile(conf.McsFuncKey)
	if err != nil {
		return nil, err
	}

	jwtConf, err := google.JWTConfigFromJSON(jsonKey)
	if err != nil {
		return nil, err
	}
	jwtConf.PrivateClaims = map[string]interface{}{
		"target_audience": conf.McsFuncURL,
	}
	jwtConf.UseIDToken = true
	mcsFuncClientCache = jwtConf.Client(oauth2.NoContext)
	mcsFuncClientCreated = time.Now()
	return mcsFuncClientCache, nil
}

func McsFuncEnabled() bool {
	return conf.McsFuncKey != "" && conf.McsFuncURL != ""
}

func McsFuncAlloc(region string) error {
	if time.Since(mcsFuncRequestTime["alloc/"+region]).Seconds() <= 30 {
		return nil
	}
	mcsFuncRequestTime["alloc/"+region] = time.Now()

	client, err := getMcsFuncClient()
	if err != nil {
		return err
	}

	resp, err := client.Get(conf.McsFuncURL + fmt.Sprintf("/alloc?region=%s&version=%s", region, gdxsvVersion))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	glog.Info(string(body))
	return nil
}

func GoMcsFuncAlloc(region string) {
	go func() {
		err := McsFuncAlloc(region)
		if err != nil {
			glog.Error(err)
		}
	}()
}
