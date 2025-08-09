# go-nginx

A high-performance HTTP server and reverse proxy implemented in Go, inspired by Nginx's architecture and features.
go-nginx provides a lightweight, extensible solution for handling HTTP requests, with built-in support for load balancing, monitoring, and graceful shutdown capabilities.

## 项目概述

Go-nginx 是一个用 Go 语言实现的高性能 HTTP 服务器和反向代理，灵感来源于 Nginx 的架构和特性。它提供了轻量级、可扩展的 HTTP 请求处理方案，内置负载均衡、监控和优雅关闭功能。

## 功能特性

- **HTTP 服务器**：全功能 HTTP/1.x 服务器，支持多路由
- **反向代理**：代理请求到后端服务，支持可配置的负载均衡
- **监控**：内置 Prometheus 监控的`/metrics`端点和服务器统计的`/stats`端点
- **优雅关闭**：正确处理 SIGINT/SIGTERM 信号，确保在关闭前完成正在进行的请求
- **YAML 配置**：通过 YAML 文件进行灵活配置
- **TLS 支持**：可选的 HTTPS 配置，用于安全连接

## 安装步骤

### 前提条件

- Go 1.16 或更高版本
- Git
- Prometheus（可选，用于指标收集）

### 从源代码安装

```bash
# 克隆仓库
git clone https://github.com/yourusername/go-nginx.git
cd go-nginx

# 构建二进制文件
go build -o go-nginx main.go

# 使用默认配置运行服务器
./go-nginx -config config.yaml
```
