package appconfig

import (
	"github.com/google/uuid"
	"github.com/ksensehq/eventnative/events"
	"github.com/ksensehq/eventnative/geo"
	"github.com/ksensehq/eventnative/logging"
	"github.com/ksensehq/eventnative/useragent"
	"github.com/spf13/viper"
	"io"
	"log"
	"strings"
)

type AppConfig struct {
	ServerName       string
	Authority        string
	AuthorizedTokens map[string]bool

	EventsConsumer events.Consumer
	GeoResolver    geo.Resolver
	UaResolver     *useragent.Resolver

	closeMe []io.Closer
}

var Instance *AppConfig

func setDefaultParams() {
	viper.SetDefault("server.port", "8001")
	viper.SetDefault("server.static_files_dir", "./web")
	viper.SetDefault("geo.maxmind_path", "/home/eventnative/app/res/")
	viper.SetDefault("log.path", "/home/eventnative/logs/events")
	viper.SetDefault("log.rotation_min", "5")
}

func Init() error {
	setDefaultParams()

	serverName := viper.GetString("server.name")
	if serverName == "" {
		serverName = "unnamed-server"
	}

	if err := logging.InitGlobalLogger(logging.Config{
		LoggerName:  "main",
		ServerName:  serverName,
		FileDir:     viper.GetString("server.log.path"),
		RotationMin: viper.GetInt64("server.log.rotation_min"),
		MaxBackups:  viper.GetInt("server.log.max_backups")}); err != nil {
		log.Fatal(err)
	}

	log.Println(" *** Creating new AppConfig *** ")
	log.Println("Server Name:", serverName)
	publicUrl := viper.GetString("server.public_url")
	if publicUrl == "" {
		log.Println("Server public url: will be taken from Host header")
	} else {
		log.Println("Server public url:", publicUrl)
	}

	var appConfig AppConfig
	appConfig.ServerName = serverName

	port := viper.GetString("port")
	if port == "" {
		port = viper.GetString("server.port")
	}
	appConfig.Authority = "0.0.0.0:" + port

	geoResolver, err := geo.CreateResolver(viper.GetString("geo.maxmind_path"))
	if err != nil {
		log.Println("Run without geo resolver", err)
	}
	appConfig.GeoResolver = geoResolver
	appConfig.UaResolver = useragent.NewResolver()

	//authorization
	// 1. from config
	tokensArr := viper.GetStringSlice("server.auth")
	authorizedTokens := map[string]bool{}
	for _, token := range tokensArr {
		if token != "" {
			authorizedTokens[strings.TrimSpace(token)] = true
		}
	}
	if len(authorizedTokens) == 0 {
		// 2. autogenerated
		generatedToken := uuid.New().String()
		authorizedTokens[generatedToken] = true
		log.Println("Empty 'server.tokens' config key. Auto generate token:", generatedToken)
	}

	appConfig.AuthorizedTokens = authorizedTokens

	//loggers per token
	writers := map[string]io.WriteCloser{}
	for token := range appConfig.AuthorizedTokens {
		eventLogWriter, err := logging.NewWriter(logging.Config{
			LoggerName:  "event-" + token,
			ServerName:  serverName,
			FileDir:     viper.GetString("log.path"),
			RotationMin: viper.GetInt64("log.rotation_min")})
		if err != nil {
			return err
		}
		writers[token] = eventLogWriter
	}

	appConfig.EventsConsumer = events.NewMultipleAsyncLogger(writers)
	appConfig.ScheduleClosing(appConfig.EventsConsumer)

	Instance = &appConfig
	return nil
}

func (a *AppConfig) ScheduleClosing(c io.Closer) {
	a.closeMe = append(a.closeMe, c)
}

func (a *AppConfig) Close() {
	for _, cl := range a.closeMe {
		if err := cl.Close(); err != nil {
			log.Println(err)
		}
	}
}
