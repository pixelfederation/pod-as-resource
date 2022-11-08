package main

// Ops represents the commandline/environment options for the program
type ConfigOpts struct {
	KubeConfig      string `long:"kubeconfig" env:"KUBECONFIG" default:"" description:"Kubeconfig path"`
	ResourceName    string `long:"resource-name" env:"RESOURCE_NAME" default:"pxfd.tech/pod-count" description:"Resource name"`
	ConfigFilePath  string `long:"config-file-path" env:"CONFIG_FILE_PATH" default:"/etc/pod-as-resource/config.yaml" description:"Absoulte path to config file"`
	DryRun          bool   `long:"dry-run" env:"DRYRUN" short:"d" description:"Dry run mode - no action taken, default: false"`
	LabelAsg        string `long:"label-asg" env:"LABEL_ASG" short:"l" default:"kops.k8s.io/instancegroup" description:"Label to determine node asg"`
	MetricsBindAddr string `long:"metrics-bind-address" short:"b" env:"METRICS_BIND_ADDRESS" default:":9898" description:"address for binding metrics listener"`
	Force           bool   `long:"force" short:"f" env:"FORCE" description:"override already patched nodes when starting"`
}
