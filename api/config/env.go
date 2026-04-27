package config

import (
	"github.com/caarlos0/env/v10"
	"github.com/joho/godotenv"
)

type Config struct {
	Env            string `env:"ENV"                   envDefault:"development"`
	Port           string `env:"PORT"                  envDefault:"8080"`
	DBHost         string `env:"DB_HOST,required"`
	DBPort         int    `env:"DB_PORT,required"`
	DBUser         string `env:"DB_USER,required"`
	DBPassword     string `env:"DB_PASSWORD,required"`
	DBName         string `env:"DB_NAME,required"`
	LogBroker      string `env:"LOG_BROKER"            envDefault:"channel"`
	AWSEndpointURL string `env:"AWS_ENDPOINT_URL"`
	AWSAccessKeyID string `env:"AWS_ACCESS_KEY_ID"    envDefault:"test"`
	AWSSecretKey   string `env:"AWS_SECRET_ACCESS_KEY" envDefault:"test"`
	AWSRegion      string `env:"AWS_REGION"            envDefault:"us-east-1"`
	S3Bucket       string `env:"S3_BUCKET"             envDefault:"brimble-deployments"`
	DockerHost     string `env:"DOCKER_HOST"           envDefault:"unix:///var/run/docker.sock"`
	DockerNetwork  string `env:"DOCKER_NETWORK"        envDefault:"brimble-paas_brimble-network"`
	Domain         string `env:"DOMAIN"                envDefault:"localhost"`
	CaddyAdminURL  string `env:"CADDY_ADMIN_URL"       envDefault:"http://localhost:2019"`
	BuildMode      string `env:"BUILD_MODE"            envDefault:"dev"` // dev | prod
	AllowedOrigins string `env:"ALLOWED_ORIGINS"       envDefault:"http://localhost:5173"`
}

func Load() *Config {
	_ = godotenv.Load()

	cfg := &Config{}
	if err := env.Parse(cfg); err != nil {
		panic("failed to load config: " + err.Error())
	}
	return cfg
}
