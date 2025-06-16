package config

import (
	"fmt"
	"github.com/ilyakaznacheev/cleanenv"
	"log"
	"sync"
)

type Config struct {
	Env            string `yaml:"env" env-default:"local"`
	TelegramApiKey string `yaml:"telegram_api_key" env-default:""`
	OpenAIApiKey   string `yaml:"openai_api_key" env-default:""`
	AssistantID    string `yaml:"assistant_id" env-default:""`
	Username       string `yaml:"username" env-default:""`
	DevPrefix      string `yaml:"dev_prefix" env-default:""`
	ImgPath        string `yaml:"img_path" env-default:""`
	Mongo          struct {
		Enabled     bool   `yaml:"enabled" env-default:"false"`
		Host        string `yaml:"host" env-default:"127.0.0.1"`
		Port        string `yaml:"port" env-default:"27017"`
		User        string `yaml:"user" env-default:"admin"`
		Password    string `yaml:"password" env-default:"pass"`
		Database    string `yaml:"database" env-default:""`
		SaveUrl     string `yaml:"save_url" env-default:""`
		ExpiredDays int    `yaml:"expired_days" env-default:"7"`
	} `yaml:"mongo"`
	ProdService struct {
		Login    string `yaml:"login" env-default:""`
		Password string `yaml:"password" env-default:""`
		BaseURL  string `yaml:"base_url" env-default:""`
	} `yaml:"prod-service"`
	Listen struct {
		BindIP string `yaml:"bind_ip" env-default:"127.0.0.1"`
		Port   string `yaml:"port" env-default:"9800"`
		ApiKey string `yaml:"key" env-default:""`
	} `yaml:"listen"`
}

var instance *Config
var once sync.Once

func MustLoad(path string) *Config {
	var err error
	once.Do(func() {
		instance = &Config{}
		if err = cleanenv.ReadConfig(path, instance); err != nil {
			desc, _ := cleanenv.GetDescription(instance, nil)
			err = fmt.Errorf("%s; %s", err, desc)
			instance = nil
			log.Fatal(err)
		}
	})
	return instance
}
