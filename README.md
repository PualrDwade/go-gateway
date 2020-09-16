# go-gateway

### 简介

go-gateway是基于golang实现的API网关服务，源码不超过300行，提供了Service登记、API登记、API网关代理调用等功能，实现简洁易懂，适合新手学习并进一步拓展

### 快速开始

#### 1.启动网关服务

gateway server端口:9000, gateway proxy端口:9001

```
go build main.go -o go-gateway & ./go-gateway
```

#### 2.注册服务与接口到网关

提供http方式进行Service与API的注册

- 注册Service

POST http://localhost:9000/createService

BODY:
```json
{
    "name":"your service name"    
    "apis": [
        {
            "name": "your api name",
            "service": "your api name",
            "protocol": "http", // or https
            "httpMethod": "GET", // or POST
            "host": "ip:port", // or domain
            "path": "your url path" // not begin with '/'
        }
    ]
}
```

- 注册API(已有service)

POST http://localhost:9000/createAPI

BODY:
```json
{
    "name": "your api name",
    "service": "your api name",
    "protocol": "http", // or https
    "httpMethod": "GET", // or POST
    "host": "ip:port", // or domain
    "path": "your url path" // not begin with '/'
}
```

#### 3.调用网关的服务接口

提供http接口调用，通过service/api的方式对go-gateway proxy发起调用

example:

API如下:
```json
{
    "name": "createUser",
    "service": "userService",
    "protocol": "http", // or https
    "httpMethod": "post", // or POST
    "host": "198.15.26.10:8080", // or domain
    "path": "user/createUser" // not begin with '/'
}
```
完成service和api注册后调用:

POST http://localhost:9001/userService/createUser

BODY: 自定义(后续增加接口参数声明)
