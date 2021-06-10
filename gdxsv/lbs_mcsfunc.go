package main

import (
	"context"
	"fmt"
	"go.uber.org/zap"
	"io/ioutil"
	"net/http"
	"sync"
	"time"

	"golang.org/x/oauth2/google"
)

var (
	mcsFuncClientCreated time.Time
	mcsFuncClientCache   *http.Client
	mtxFuncRequestTime   sync.Mutex
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
	"europe-west6":            "Zurich, Switzerland",
	"northamerica-northeast1": "Montreal, Quebec, Canada",
	"southamerica-east1":      "Osasco (Sao Paulo), Brazil",
	"us-central1":             "Council Bluffs, Iowa, USA",
	"us-east1":                "Moncks Corner, South Carolina, USA",
	"us-east4":                "Ashburn, Northern Virginia, USA",
	"us-west1":                "The Dalles, Oregon, USA",
	"us-west2":                "Los Angeles, California, USA",
	"us-west3":                "Salt Lake City, Utah, USA",
}

var gcpRegionGroup = map[string]string{
	"asia-east1":              "asia-east",
	"asia-east2":              "asia-east",
	"asia-northeast1":         "asia-northeast",
	"asia-northeast2":         "asia-northeast",
	"asia-northeast3":         "asia-northeast",
	"asia-south1":             "asia-south",
	"asia-southeast1":         "asia-southeast",
	"australia-southeast1":    "australia-southeast",
	"europe-north1":           "europe-north",
	"europe-west1":            "europe-west",
	"europe-west2":            "europe-west",
	"europe-west3":            "europe-west",
	"europe-west4":            "europe-west",
	"europe-west6":            "europe-west",
	"northamerica-northeast1": "northamerica-northeast",
	"southamerica-east1":      "southamerica-east",
	"us-central1":             "us-central",
	"us-east1":                "us-east",
	"us-east4":                "us-east",
	"us-west1":                "us-west",
	"us-west2":                "us-west",
	"us-west3":                "us-west",
}

func getMcsFuncClient() (*http.Client, error) {
	if mcsFuncClientCache != nil && time.Since(mcsFuncClientCreated).Minutes() <= 30.0 {
		return mcsFuncClientCache, nil
	}

	jsonKey, err := ioutil.ReadFile(conf.GCPKeyPath)
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
	mcsFuncClientCache = jwtConf.Client(context.Background())
	mcsFuncClientCreated = time.Now()
	return mcsFuncClientCache, nil
}

func McsFuncEnabled() bool {
	return conf.GCPKeyPath != "" && conf.McsFuncURL != ""
}

func McsFuncAlloc(region string) error {
	mtxFuncRequestTime.Lock()
	if time.Since(mcsFuncRequestTime["alloc/"+region]).Seconds() <= 30 {
		mtxFuncRequestTime.Unlock()
		return nil
	}
	mcsFuncRequestTime["alloc/"+region] = time.Now()
	mtxFuncRequestTime.Unlock()

	client, err := getMcsFuncClient()
	if err != nil {
		return err
	}

	resp, err := client.Get(conf.McsFuncURL + fmt.Sprintf("/alloc?region=%s&version=%s", region, "latest"))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	logger.Info("mcsfunc alloc", zap.ByteString("response", body))
	return nil
}

func GoMcsFuncAlloc(region string) bool {
	mtxFuncRequestTime.Lock()
	if time.Since(mcsFuncRequestTime["alloc/"+region]).Seconds() <= 30 {
		mtxFuncRequestTime.Unlock()
		return false
	}
	mtxFuncRequestTime.Unlock()

	go func() {
		err := McsFuncAlloc(region)
		if err != nil {
			logger.Error("mcsfunc alloc failed", zap.Error(err))
		}
	}()

	return true
}
