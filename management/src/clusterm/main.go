package main

import (
	"bufio"
	"fmt"
	"io"
	"os"

	"github.com/Sirupsen/logrus"
	"github.com/codegangsta/cli"
	"github.com/contiv/cluster/management/src/clusterm/manager"
	"github.com/contiv/errored"
)

// version is provided by build
var version = ""

type logLevel struct {
	value logrus.Level
}

func (l *logLevel) Set(value string) error {
	var err error
	if l.value, err = logrus.ParseLevel(value); err != nil {
		return err
	}
	return nil
}

func (l *logLevel) String() string {
	return l.value.String()
}

func (l *logLevel) usage() string {
	return fmt.Sprintf("debug trace level: %s, %s, %s, %s, %s or %s", logrus.PanicLevel,
		logrus.FatalLevel, logrus.ErrorLevel, logrus.WarnLevel, logrus.InfoLevel, logrus.DebugLevel)
}

func main() {
	app := cli.NewApp()
	app.Name = os.Args[0]
	app.Usage = "cluster manager daemon"
	app.Version = version
	app.Flags = []cli.Flag{
		cli.GenericFlag{
			Name:  "debug",
			Value: &logLevel{value: logrus.DebugLevel},
			Usage: (&logLevel{}).usage(),
		},
		cli.StringFlag{
			Name:  "config",
			Value: "",
			Usage: "read cluster manager's configuration from file. Use '-' to read configuration from stdin",
		},
	}
	app.Action = startDaemon

	app.Run(os.Args)
}

func getConfig(c *cli.Context) (*manager.Config, string, error) {
	var reader io.Reader
	configFile := ""
	if !c.GlobalIsSet("config") {
		logrus.Debugf("no configuration was specified, starting with default.")
	} else if c.GlobalString("config") == "-" {
		logrus.Debugf("reading configuration from stdin")
		reader = bufio.NewReader(os.Stdin)
	} else {
		f, err := os.Open(c.GlobalString("config"))
		if err != nil {
			return nil, "", errored.Errorf("failed to open config file. Error: %v", err)
		}
		defer func() { f.Close() }()
		logrus.Debugf("reading configuration from file: %q", c.GlobalString("config"))
		reader = bufio.NewReader(f)
		configFile = c.GlobalString("config")
	}
	config := manager.DefaultConfig()
	if reader != nil {
		if _, err := config.MergeFromReader(reader); err != nil {
			return nil, "", errored.Errorf("failed to merge configuration. Error: %v", err)
		}
	}
	return config, configFile, nil
}

func startDaemon(c *cli.Context) {
	// set log level
	level := c.GlobalGeneric("debug").(*logLevel)
	logrus.SetLevel(level.value)
	logrus.SetFormatter(&logrus.TextFormatter{DisableTimestamp: true})

	config, configFile, err := getConfig(c)
	if err != nil {
		logrus.Fatalf("failed to get configuration. Error: %v", err)
	}

	mgr, err := manager.NewManager(config, configFile)
	if err != nil {
		logrus.Fatalf("failed to initialize the manager. Error: %s", err)
	}

	// start manager's processing loop
	errCh := make(chan error, 5)
	go mgr.Run(errCh)
	select {
	case err := <-errCh:
		logrus.Fatalf("encountered an error: %s", err)
	}
}
