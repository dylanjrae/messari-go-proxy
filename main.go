package main

import (
	"github.com/gin-gonic/gin"
	"net/http"
	"io/ioutil"
	"encoding/json"
	"net/http/httputil"
	"log"
	"fmt"
	"time"
	"net/url"
	"bytes"
	"strings"
)

type DebugResponseData struct {
	ReverseProxyUrlMessage string `json:"reverse proxy url"`
	ApiKeyMessage string `json:"your api key"`
	PathMessage string `json:"your path"`
}

type PrettyProxyWrapper struct {
	Label map[string]interface{} `json:"messari_pretty_proxy_v0.1"`
}

// Custom struct to preserve JSON key order
type OrderedMap struct {
	Keys   []string
	Values map[string]json.RawMessage
}

var baseMessariUrl string = "https://data.messari.io/api"
var client http.Client

func buildMessariRequest(method, requestUrl string, apiKey string) (*http.Request) {
	req, err := http.NewRequest("GET", baseMessariUrl + requestUrl, nil)
	
	if (err != nil) {
		log.Fatal(err)
	}

	req.Header.Set("x-messari-api-key", apiKey)
	// req.Header.Set("Content-Type", "application/json")
	

	return req
}

func fetchMessariRequest(preparedMessariRequest *http.Request) (*http.Response) {
	res, err := client.Do(preparedMessariRequest)

	if (err != nil) {
		log.Fatal(err)
	}

	return res
}

func manageProxyDirector(proxy *httputil.ReverseProxy, req *http.Request, target *url.URL, messariRequestPath string) {
	proxy.Director = func(req *http.Request) {
		req.URL.Scheme = target.Scheme
		req.URL.Host = target.Host
		req.URL.Path = messariRequestPath
	}
}

func modifyResponse(proxy *httputil.ReverseProxy) {
	proxy.ModifyResponse = func(res *http.Response) error {
		body, err := ioutil.ReadAll(res.Body)
		if err != nil {
			return err
		}

		var jsonData map[string]interface{}
		err = json.Unmarshal(body, &jsonData)
		if err != nil {
			log.Println("Error parsing JSON:", err)
			return err
		}

		// Traverse the JSON and add a new field below any field containing "price"
		addNewField(jsonData)

		newData := &PrettyProxyWrapper{
			Label:    jsonData,
		}

		newJsonData, err := json.Marshal(newData)
		if err != nil {
			log.Println("Error marshaling JSON:", err)
			return err
		}

		res.Body = ioutil.NopCloser(bytes.NewBuffer(newJsonData))

		// Update the Content-Length header
		res.ContentLength = int64(len(newJsonData))

		return nil
	}
}

func addNewField(data map[string]interface{}) { //, parentKey string) {
	for key, value := range data {
		switch v := value.(type) {
		case map[string]interface{}:
			addNewField(v) // Recursively traverse nested objects
		case float64:
			if strings.Contains(key, "price") {
				// Add a new field below the "price" field
				data["pretty_" + key] = prettifyPrice(v)
			}
		}
	}
}

func prettifyPrice(price float64) string {
	return fmt.Sprintf("$ %.2f", price)
}

// func addFieldBelowPrice(data map[string]interface{}, key string, parentKey string) {
// 	newKey := key + "_new"
// 	if parentKey != "" {
// 		newKey = parentKey + "." + newKey
// 	}

// 	for k := range data {
// 		if k == key {
// 			// Move the existing field to a temporary key
// 			data["__temp__"] = data[k]
// 			delete(data, k)
// 			break
// 		}
// 	}

// 	// Add the new field directly below the "price" field
// 	data[newKey] = "New value"

// 	// Restore the existing field to its original key
// 	data[key] = data["__temp__"]
// 	delete(data, "__temp__")
// }

func main() {
	target,_ := url.Parse(baseMessariUrl)
	backendProxy := httputil.NewSingleHostReverseProxy(target)
	
	router := gin.Default()

	router.GET("/*messariPath", func(c *gin.Context) {
		messariApiKey := c.Request.Header.Get("x-messari-api-key")
		messariRequestPath := c.Request.URL.Path
		debugData := DebugResponseData{
			ApiKeyMessage: messariApiKey, 
			PathMessage: messariRequestPath,
			ReverseProxyUrlMessage: baseMessariUrl,
		}
		
		if(c.Query("x-debug") == "true") {
			c.JSON(200, debugData)
			return
		}

		preparedMessariRequest := buildMessariRequest(http.MethodGet, messariRequestPath, messariApiKey)

		if(c.Query("x-disable-all-features") == "true") {
			log.Println("here")
			manageProxyDirector(backendProxy, c.Request, target, messariRequestPath)
			backendProxy.ServeHTTP(c.Writer, preparedMessariRequest)
			return
		}

		start := time.Now()
		manageProxyDirector(backendProxy, c.Request, target, messariRequestPath)
		modifyResponse(backendProxy)
		backendProxy.ServeHTTP(c.Writer, preparedMessariRequest)
		end := time.Since(start)
		log.Println("Request to messari took: ", end)		
	})

	router.Run(":8080")

}