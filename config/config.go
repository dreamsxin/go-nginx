package config

import (
	"time"

	"github.com/spf13/viper"
)

// Config 应用配置结构
type Config struct {
	Server   ServerConfig   `mapstructure:"server"`
	Log      LogConfig      `mapstructure:"log"`
	Balancer BalancerConfig `mapstructure:"balancer"`
}

// ServerConfig 服务器配置
type ServerConfig struct {
	Port       int           `mapstructure:"port"`
	Workers    int           `mapstructure:"workers"`
	Timeout    ServerTimeout `mapstructure:"timeout"`
	Monitoring struct {
		Enabled     bool   `yaml:"enabled"`
		MetricsPath string `yaml:"metrics_path"`
		MetricsPort int    `yaml:"metrics_port"`
	} `yaml:"monitoring"`
}

// ServerTimeout 超时配置
type ServerTimeout struct {
	Read  time.Duration `mapstructure:"read"`
	Write time.Duration `mapstructure:"write"`
	Idle  time.Duration `mapstructure:"idle"`
}

// LogConfig 日志配置
type LogConfig struct {
	Level  string `mapstructure:"level"`
	Output string `mapstructure:"output"`
	File   string `mapstructure:"file"`
}

// LoadConfig 加载配置文件
func LoadConfig(path string) (*Config, error) {
	viper.SetConfigFile(path)
	viper.SetConfigType("yaml")

	if err := viper.ReadInConfig(); err != nil {
		return nil, err
	}

	var config Config
	if err := viper.Unmarshal(&config); err != nil {
		return nil, err
	}

	return &config, nil
}

// 负载均衡配置
type BalancerConfig struct {
	HTTP HTTPBalancerConfig `mapstructure:"http"`
	TCP  TCPBalancerConfig  `mapstructure:"tcp"`
	UDP  UDPBalancerConfig  `mapstructure:"udp"`
}

// HTTP负载均衡配置
type HTTPBalancerConfig struct {
	Enabled            bool                     `mapstructure:"enabled"`
	Listen             int                      `mapstructure:"listen"`
	Strategy           string                   `mapstructure:"strategy"`
	SessionPersistence SessionPersistenceConfig `mapstructure:"session_persistence"`
	Backends           []BackendConfig          `mapstructure:"backends"`
	HealthCheck        HealthCheckConfig        `mapstructure:"health_check"`
}

// TCP负载均衡配置
type TCPBalancerConfig struct {
	Enabled     bool              `mapstructure:"enabled"`
	Listen      int               `mapstructure:"listen"`
	Strategy    string            `mapstructure:"strategy"`
	Backends    []BackendConfig   `mapstructure:"backends"`
	HealthCheck HealthCheckConfig `mapstructure:"health_check"` // 添加健康检查配置
}

// UDP负载均衡配置
type UDPBalancerConfig struct {
	Enabled     bool              `mapstructure:"enabled"`
	Listen      int               `mapstructure:"listen"`
	Strategy    string            `mapstructure:"strategy"`
	Backends    []BackendConfig   `mapstructure:"backends"`
	HealthCheck HealthCheckConfig `mapstructure:"health_check"`
}

// 会话保持配置
type SessionPersistenceConfig struct {
	Enabled      bool          `mapstructure:"enabled"`
	Type         string        `mapstructure:"type"`
	CookieName   string        `mapstructure:"cookie_name"`
	CookieMaxAge time.Duration `mapstructure:"cookie_max_age"`
}

// 后端服务器配置
type BackendConfig struct {
	Name        string        `mapstructure:"name"`
	Address     string        `mapstructure:"address"`
	Weight      int           `mapstructure:"weight"`
	MaxFails    int           `mapstructure:"max_fails"`
	FailTimeout time.Duration `mapstructure:"fail_timeout"`
}

// 健康检查配置
type HealthCheckConfig struct {
	Enabled            bool          `mapstructure:"enabled"`
	Interval           time.Duration `mapstructure:"interval"`
	Timeout            time.Duration `mapstructure:"timeout"`
	MaxFails           int           `mapstructure:"max_fails"`
	FailTimeout        time.Duration `mapstructure:"fail_timeout"`
	HealthyThreshold   int           `mapstructure:"healthy_threshold"`
	UnhealthyThreshold int           `mapstructure:"unhealthy_threshold"`
	Send               string        `mapstructure:"send"`
	Receive            string        `mapstructure:"receive"`
}
