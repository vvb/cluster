package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strings"
	"text/tabwriter"

	log "github.com/Sirupsen/logrus"
	"github.com/codegangsta/cli"
	"github.com/contiv/cluster/management/src/clusterm/manager"
	"github.com/contiv/errored"
	"github.com/jmoiron/jsonq"
)

var (
	clustermFlags = []cli.Flag{
		cli.StringFlag{
			Name:  "url, u",
			Value: manager.DefaultConfig().Manager.Addr,
			Usage: "cluster manager's REST service url",
		},
	}

	extraVarsFlags = []cli.Flag{
		cli.StringFlag{
			Name:  "extra-vars, e",
			Value: "",
			Usage: "extra vars for ansible configuration. This should be a quoted json string.",
		},
	}

	statusFieldsFlags = []cli.Flag{
		cli.StringFlag{
			Name:  "fields, f",
			Value: "",
			Usage: "Comma separated string of fields to display as part of status. Supported Values: Name, State, Status, PrevStatus, PrevState, InventoryName, HostGroup, SshAddress, Label, SerialNumber, ManagementAddress ",
		},
	}
)

func main() {
	app := cli.NewApp()
	app.Name = os.Args[0]
	app.Usage = "utility to interact with cluster manager"
	app.Flags = clustermFlags
	app.Commands = []cli.Command{
		{
			Name:    "node",
			Aliases: []string{"n"},
			Usage:   "node related operation",
			Subcommands: []cli.Command{
				{
					Name:    "commission",
					Aliases: []string{"c"},
					Usage:   "commission a node",
					Action:  doAction(newPostActioner(validateOneNodeName, nodeCommission)),
					Flags:   extraVarsFlags,
				},
				{
					Name:    "decommission",
					Aliases: []string{"d"},
					Usage:   "decommission a node",
					Action:  doAction(newPostActioner(validateOneNodeName, nodeDecommission)),
					Flags:   extraVarsFlags,
				},
				{
					Name:    "maintenance",
					Aliases: []string{"m"},
					Usage:   "put a node in maintenance",
					Action:  doAction(newPostActioner(validateOneNodeName, nodeMaintenance)),
					Flags:   extraVarsFlags,
				},
				{
					Name:    "get",
					Aliases: []string{"g"},
					Usage:   "get node's status information",
					Action:  doAction(newGetActioner(nodeGet)),
				},
				{
					Name:    "status",
					Aliases: []string{"s"},
					Usage:   "get node's status summary",
					Action:  doAction(newGetStatusActioner(nodeStatus)),
					Flags:   statusFieldsFlags,
				},
			},
		},
		{
			Name:    "nodes",
			Aliases: []string{"a"},
			Usage:   "all nodes related operation",
			Subcommands: []cli.Command{
				{
					Name:    "commission",
					Aliases: []string{"c"},
					Usage:   "commission a set of nodes",
					Action:  doAction(newPostActioner(validateMultiNodeNames, nodesCommission)),
					Flags:   extraVarsFlags,
				},
				{
					Name:    "decommission",
					Aliases: []string{"d"},
					Usage:   "decommission a set of nodes",
					Action:  doAction(newPostActioner(validateMultiNodeNames, nodesDecommission)),
					Flags:   extraVarsFlags,
				},
				{
					Name:    "maintenance",
					Aliases: []string{"m"},
					Usage:   "put a set of nodes in maintenance",
					Action:  doAction(newPostActioner(validateMultiNodeNames, nodesMaintenance)),
					Flags:   extraVarsFlags,
				},
				{
					Name:    "get",
					Aliases: []string{"g"},
					Usage:   "get status information for all nodes",
					Action:  doAction(newGetActioner(nodesGet)),
				},
				{
					Name:    "status",
					Aliases: []string{"s"},
					Usage:   "get node's status summary",
					Action:  doAction(newGetStatusActioner(nodesStatus)),
					Flags:   statusFieldsFlags,
				},
			},
		},
		{
			Name:    "global",
			Aliases: []string{"g"},
			Usage:   "set/get global info",
			Subcommands: []cli.Command{
				{
					Name:    "get",
					Aliases: []string{"g"},
					Usage:   "get global info",
					Action:  doAction(newGetActioner(globalsGet)),
				},
				{
					Name:    "set",
					Aliases: []string{"s"},
					Usage:   "set global info",
					Flags:   extraVarsFlags,
					Action:  doAction(newPostActioner(validateZeroArgs, globalsSet)),
				},
			},
		},
		{
			Name:    "discover",
			Aliases: []string{"d"},
			Usage:   "provision one or more nodes for discovery",
			Action:  doAction(newPostActioner(validateMultiNodeAddrs, nodesDiscover)),
			Flags:   extraVarsFlags,
		},
	}

	app.Run(os.Args)
}

func errUnexpectedArgCount(exptd string, rcvd int) error {
	return errored.Errorf("command expects %s arg(s) but received %d", exptd, rcvd)
}

func errInvalidIPAddr(a string) error {
	return errored.Errorf("failed to parse ip address %q", a)
}

type actioner interface {
	procFlags(*cli.Context)
	procArgs(*cli.Context)
	action(*manager.Client) error
}

func doAction(a actioner) func(*cli.Context) {
	return func(c *cli.Context) {
		cClient := manager.NewClient(c.GlobalString("url"))
		a.procArgs(c)
		a.procFlags(c)
		if err := a.action(cClient); err != nil {
			log.Fatalf(err.Error())
		}
	}
}

type postCallback func(c *manager.Client, args []string, extraVars string) error
type validateCallback func(args []string) error

type postActioner struct {
	args       []string
	extraVars  string
	validateCb validateCallback
	postCb     postCallback
}

func newPostActioner(validateCb validateCallback, postCb postCallback) *postActioner {
	return &postActioner{
		validateCb: validateCb,
		postCb:     postCb,
	}
}

func (npa *postActioner) procFlags(c *cli.Context) {
	npa.extraVars = c.String("extra-vars")
}

func (npa *postActioner) procArgs(c *cli.Context) {
	npa.args = c.Args()
}

func (npa *postActioner) action(c *manager.Client) error {
	if err := npa.validateCb(npa.args); err != nil {
		return err
	}
	return npa.postCb(c, npa.args, npa.extraVars)
}

func validateOneNodeName(args []string) error {
	if len(args) != 1 {
		return errUnexpectedArgCount("1", len(args))
	}
	return nil
}

func nodeCommission(c *manager.Client, args []string, extraVars string) error {
	nodeName := args[0]
	return c.PostNodeCommission(nodeName, extraVars)
}

func nodeDecommission(c *manager.Client, args []string, extraVars string) error {
	nodeName := args[0]
	return c.PostNodeDecommission(nodeName, extraVars)
}

func nodeMaintenance(c *manager.Client, args []string, extraVars string) error {
	nodeName := args[0]
	return c.PostNodeInMaintenance(nodeName, extraVars)
}

func validateMultiNodeNames(args []string) error {
	if len(args) < 1 {
		return errUnexpectedArgCount(">=1", len(args))
	}
	return nil
}

func nodesCommission(c *manager.Client, args []string, extraVars string) error {
	return c.PostNodesCommission(args, extraVars)
}

func nodesDecommission(c *manager.Client, args []string, extraVars string) error {
	return c.PostNodesDecommission(args, extraVars)
}

func nodesMaintenance(c *manager.Client, args []string, extraVars string) error {
	return c.PostNodesInMaintenance(args, extraVars)
}

func validateMultiNodeAddrs(args []string) error {
	if len(args) < 1 {
		return errUnexpectedArgCount(">=1", len(args))
	}
	for _, addr := range args {
		if ip := net.ParseIP(addr); ip == nil {
			return errInvalidIPAddr(addr)
		}
	}
	return nil
}

func nodesDiscover(c *manager.Client, args []string, extraVars string) error {
	return c.PostNodesDiscover(args, extraVars)
}

func validateZeroArgs(args []string) error {
	if len(args) != 0 {
		return errUnexpectedArgCount("0", len(args))
	}
	return nil
}

func globalsSet(c *manager.Client, noop []string, extraVars string) error {
	return c.PostGlobals(extraVars)
}

type getActioner struct {
	nodeName string
	getCb    func(c *manager.Client, nodeName string) error
}

func newGetActioner(getCb func(c *manager.Client, nodeName string) error) *getActioner {
	return &getActioner{getCb: getCb}
}

func (nga *getActioner) procFlags(c *cli.Context) {
	return
}

func (nga *getActioner) procArgs(c *cli.Context) {
	nga.nodeName = c.Args().First()
}

func (nga *getActioner) action(c *manager.Client) error {
	return nga.getCb(c, nga.nodeName)
}

func nodeGet(c *manager.Client, nodeName string) error {
	if nodeName == "" {
		return errUnexpectedArgCount("1", 0)
	}

	out, err := c.GetNode(nodeName)
	if err != nil {
		return err
	}

	var outBuf bytes.Buffer
	json.Indent(&outBuf, out, "", "    ")
	outBuf.WriteTo(os.Stdout)
	return nil
}

func nodesGet(c *manager.Client, noop string) error {
	out, err := c.GetAllNodes()
	if err != nil {
		return err
	}

	var outBuf bytes.Buffer
	json.Indent(&outBuf, out, "", "    ")
	outBuf.WriteTo(os.Stdout)
	return nil
}

type getStatusActioner struct {
	nodeName string
	fields   string
	getCb    func(c *manager.Client, nodeName, fields string) error
}

func newGetStatusActioner(getCb func(c *manager.Client, nodeName, fields string) error) *getStatusActioner {
	return &getStatusActioner{getCb: getCb}
}

func (nga *getStatusActioner) procFlags(c *cli.Context) {
	nga.fields = c.String("fields")
	return
}

func (nga *getStatusActioner) procArgs(c *cli.Context) {
	nga.nodeName = c.Args().First()
}

func (nga *getStatusActioner) action(c *manager.Client) error {
	return nga.getCb(c, nga.nodeName, nga.fields)
}

func nodeStatus(c *manager.Client, nodeName, fields string) error {
	if nodeName == "" {
		return errUnexpectedArgCount("1", 0)
	}

	out, err := c.GetNode(nodeName)
	if err != nil {
		return err
	}

	printNodesStatus(out, fields)
	return nil
}

func nodesStatus(c *manager.Client, noop, fields string) error {
	out, err := c.GetAllNodes()
	if err != nil {
		return err
	}
	printNodesStatus(out, fields)
	return nil
}

func printNodesStatus(data []byte, fields string) {
	m := make(map[string]interface{})
	if err := json.Unmarshal(data, &m); err != nil {
		fmt.Println(err)
	}

	headers := "Name,State,Status,HostGroup"
	for _, each := range strings.Split(fields, ",") {
		headers += "," + strings.Trim(each, " ")
	}
	hdrs := strings.Split(headers, ",")

	var format []string
	var args []interface{}
	for _, each := range hdrs {
		format = append(format, "%s")
		args = append(args, each)
	}

	writer := tabwriter.NewWriter(os.Stdout, 0, 8, 8, ' ', 0)

	writer.Write([]byte(fmt.Sprintf(strings.Join(format, "|"), args...)))
	writer.Write([]byte("\r\n"))
	writer.Write([]byte("-------------------------------------------------------------------------------------------------------------------------------"))
	writer.Write([]byte("\r\n"))
	//var outBuf bytes.Buffer
	//outBuf.WriteString(fmt.Sprintf(strings.Join(format, "|"), args...))
	//outBuf.WriteString("\r\n")
	//outBuf.WriteString("-------------------------------------------------------------------------------------------------------------------------------")
	//outBuf.WriteString("\r\n")

	jq := getJSONDecoderObj(data)

	// detect is this is node vs nodes
	if _, ok := m["inventory-state"]; ok {
		invObj, _ := jq.Object("inventory-state")
		cfgObj, _ := jq.Object("configuration-state")
		monObj, _ := jq.Object("monitoring-state")

		d := extractDisplayVars(hdrs, invObj, cfgObj, monObj)
		//outBuf.WriteString(fmt.Sprintf(strings.Join(format, "|"), d...))
		//outBuf.WriteString("\r\n")
		//outBuf.WriteTo(os.Stdout)
		writer.Write([]byte(fmt.Sprintf(strings.Join(format, "|"), d...)))
		writer.Write([]byte("\r\n"))
		writer.Flush()
		return
	}

	for s := range m {
		invObj, _ := jq.Object(s, "inventory-state")
		cfgObj, _ := jq.Object(s, "configuration-state")
		monObj, _ := jq.Object(s, "monitoring-state")
		d := extractDisplayVars(hdrs, invObj, cfgObj, monObj)
		//outBuf.WriteString(fmt.Sprintf(strings.Join(format, "|"), d...))
		//outBuf.WriteString("\r\n")
		writer.Write([]byte(fmt.Sprintf(strings.Join(format, "|"), d...)))
		writer.Write([]byte("\r\n"))
	}

	writer.Flush()
	//outBuf.WriteTo(os.Stdout)
	return
}

func extractDisplayVars(fields []string, invObj, cfgObj, monObj map[string]interface{}) []interface{} {
	d := map[string]string{
		"Name":       invObj["name"].(string),
		"State":      invObj["state"].(string),
		"Status":     invObj["status"].(string),
		"PrevStatus": invObj["prev-status"].(string),
		"PrevState":  invObj["prev-state"].(string),

		"InventoryName": cfgObj["inventory-name"].(string),
		"HostGroup":     cfgObj["host-group"].(string),
		"SshAddress":    cfgObj["ssh-address"].(string),

		"Label":             monObj["label"].(string),
		"SerialNumber":      monObj["serial-number"].(string),
		"ManagementAddress": monObj["management-address"].(string),
	}

	var displayVars []interface{}
	for _, each := range fields {
		displayVars = append(displayVars, d[each])
	}

	return displayVars
}

func getJSONDecoderObj(data []byte) *jsonq.JsonQuery {
	m := map[string]interface{}{}
	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.Decode(&m)
	jq := jsonq.NewQuery(m)
	return jq
}

func globalsGet(c *manager.Client, noop string) error {
	out, err := c.GetGlobals()
	if err != nil {
		return err
	}

	var outBuf bytes.Buffer
	json.Indent(&outBuf, out, "", "    ")
	outBuf.WriteTo(os.Stdout)
	return nil
}
