package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httputil"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
)

// Service define the api collections
type Service struct {
	Name string          `json:"name"`
	APIs map[string]*API `json:"apis"`
}

// API define the api object
type API struct {
	Name       string `json:"name"`       // api name
	Service    string `json:"service"`    // service name
	Protocol   string `json:"protocol"`   // http or https
	HTTPMethod string `json:"httpMethod"` // http method
	Host       string `json:"host"`       // ip:port or domain
	Path       string `json:"path"`       // request path
}

// Discovery discovery the service by service name
type Discovery interface {
	// GetService get service by serviceName
	GetService(serviceName string) (*Service, error)
	// CreateService create new service
	CreateService(service *Service) error
	// CreateAPI create api object for given serviceName
	CreateAPI(api *API) error
}

// cache implements Discovery interface used local store
type cache struct {
	store map[string]*Service
	mu    sync.RWMutex
}

// NewCacheDiscovery return cache implements fot Discovery
func NewCacheDiscovery() Discovery {
	return &cache{store: make(map[string]*Service), mu: sync.RWMutex{}}
}

// GetService get service by serviceName
func (c *cache) GetService(serviceName string) (*Service, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	service, exist := c.store[serviceName]
	if !exist {
		return nil, fmt.Errorf("service: %v not exist", serviceName)
	}
	return service, nil
}

// CreateService create new service
func (c *cache) CreateService(service *Service) error {
	if service == nil || service.Name == "" {
		return fmt.Errorf("service can not be empty")
	}
	// not allow duplicate service with samename
	c.mu.Lock()
	defer c.mu.Unlock()
	_, exist := c.store[service.Name]
	if exist {
		return fmt.Errorf("service: %v already exist", service.Name)
	}
	// add to cache store
	c.store[service.Name] = service
	return nil
}

// CreateAPI create api object for given serviceName
func (c *cache) CreateAPI(api *API) error {
	if api == nil || api.Name == "" {
		return fmt.Errorf("api can not be empty")
	}
	serviceName := api.Service
	if serviceName == "" {
		return fmt.Errorf("service name can not be empty")
	}
	// not allow duplicate service with samename
	c.mu.Lock()
	defer c.mu.Unlock()
	service, exist := c.store[serviceName]
	if !exist {
		return fmt.Errorf("service: %v not exist", service.Name)
	}
	_, exist = service.APIs[api.Name]
	if exist {
		return fmt.Errorf("service: %v, api: %v already exist", serviceName, api.Name)
	}
	// add api to cache store
	service.APIs[api.Name] = api
	return nil
}

// APIGateway control the access to backend service and apis
type APIGateway struct {
	directorFunc func(req *http.Request)
	discovery    Discovery
}

// NewAPIGateWay create instructed api gateway to handle user request
func NewAPIGateWay() *APIGateway {
	gateway := &APIGateway{}
	// 1. register service discovery to gateway
	gateway.discovery = NewCacheDiscovery()
	// 2. register director to gateway for reserve proxy
	director := func(req *http.Request) {
		// request just as: /{servicename}/{apiname}
		reqPath := req.URL.Path
		if reqPath == "" {
			return
		}
		pathArray := strings.Split(reqPath, "/")
		if len(pathArray) != 3 {
			log.Printf("request path: %v format error", reqPath)
			return
		}
		serviceName := pathArray[1]
		apiName := pathArray[2]
		log.Printf("request service name: %v, api name: %v", serviceName, apiName)

		// use service discovery
		service, err := gateway.discovery.GetService(serviceName)
		if err != nil {
			log.Printf("use discovery to get service failed: %v\n", err)
			return
		}
		// reorgnize request to true api backend
		api, exist := service.APIs[apiName]
		if !exist {
			log.Printf("service: %v not has api: %v\n", serviceName, apiName)
			return
		}
		// set api backend info
		req.URL.Scheme = api.Protocol
		req.URL.Host = api.Host
		req.URL.Path = "/" + api.Path
	}
	gateway.directorFunc = director
	return gateway
}

func (gateway *APIGateway) director(req *http.Request) {
	// request just as: /{servicename}/{apiname}
	reqPath := req.URL.Path
	if reqPath == "" {
		return
	}
	pathArray := strings.Split(reqPath, "/")
	if len(pathArray) != 3 {
		return
	}
	serviceName := pathArray[1]
	apiName := pathArray[2]
	log.Printf("request service name: %v, api name: %v", serviceName, apiName)

	// use service discovery
	service, err := gateway.discovery.GetService(serviceName)
	if err != nil {
		log.Printf("use discovery to get service failed: %v\n", err)
		return
	}
	// reorgnize request to true api backend
	api, exist := service.APIs[apiName]
	if !exist {
		log.Printf("service: %v not has api: %v\n", serviceName, apiName)
		return
	}
	if req.Method != api.HTTPMethod {
		log.Printf("method: %v unsupported, should be: %v", req.Method, api.HTTPMethod)
	}
	// set api backend info
	req.URL.Scheme = api.Protocol
	req.URL.Host = api.Host
	req.URL.Path = "/" + api.Path
}

// ServeHTTP use gateway as a handler
func (gateway *APIGateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	proxy := &httputil.ReverseProxy{
		Director: gateway.director,
	}
	proxy.ServeHTTP(w, r)
}

// RunServer start to provide native api for service/api operations
func (gateway *APIGateway) RunServer() {
	serverPort := ":9000"
	mux := http.NewServeMux()
	mux.HandleFunc("/createService", gateway.CreateService)
	mux.HandleFunc("/createAPI", gateway.CreateAPI)
	log.Printf("gateway server started at http://localhost%v", serverPort)
	if err := http.ListenAndServe(serverPort, mux); err != nil {
		log.Fatal(err)
	}
}

// RunProxy start to reserve proxy user request
func (gateway *APIGateway) RunProxy() {
	proxyPort := ":9001"
	log.Printf("gateway proxy started at http://localhost%v", proxyPort)
	if err := http.ListenAndServe(proxyPort, gateway); err != nil {
		log.Fatal(err)
	}
}

// CreateService handle http request to register service
func (gateway *APIGateway) CreateService(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Write([]byte(fmt.Sprintf("http method %v not support", r.Method)))
		return
	}
	data, _ := ioutil.ReadAll(r.Body)
	defer r.Body.Close()
	var service Service
	err := json.Unmarshal(data, &service)
	if err != nil {
		w.Write([]byte(fmt.Sprintf("unmarshal request body failed: %v", err)))
		return
	}
	err = gateway.discovery.CreateService(&service)
	if err != nil {
		w.Write([]byte(fmt.Sprintf("create service failed: %v", err)))
		return
	}
	w.Write([]byte("success"))
}

// CreateAPI handle http request to register service api
func (gateway *APIGateway) CreateAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Write([]byte(fmt.Sprintf("http method %v not support", r.Method)))
		return
	}
	data, _ := ioutil.ReadAll(r.Body)
	defer r.Body.Close()
	var api API
	err := json.Unmarshal(data, &api)
	if err != nil {
		w.Write([]byte(fmt.Sprintf("unmarshal request body failed: %v", err)))
		return
	}
	err = gateway.discovery.CreateAPI(&api)
	if err != nil {
		w.Write([]byte(fmt.Sprintf("create api failed: %v", err)))
		return
	}
	w.Write([]byte("success"))
}

func main() {
	apigateway := NewAPIGateWay()
	go func() {
		apigateway.RunProxy()
	}()
	go func() {
		apigateway.RunServer()
	}()
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)
	<-signalChan
	log.Println("got os shutdown signal, shutting down go-gateway server gracefully...")
}
