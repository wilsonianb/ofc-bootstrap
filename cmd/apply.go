// Copyright (c) OpenFaaS Author(s) 2020. All rights reserved.
// Licensed under the MIT license. See LICENSE file in the project root for full license information.

package cmd

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/alexellis/arkade/pkg/config"
	"github.com/alexellis/arkade/pkg/env"
	"github.com/alexellis/arkade/pkg/get"
	"github.com/alexellis/arkade/pkg/k8s"
	execute "github.com/alexellis/go-execute/pkg/v1"
	"github.com/openfaas/ofc-bootstrap/pkg/ingress"
	"github.com/openfaas/ofc-bootstrap/pkg/stack"
	"github.com/openfaas/ofc-bootstrap/pkg/tls"

	"github.com/openfaas/ofc-bootstrap/pkg/types"
	yaml "gopkg.in/yaml.v2"
)

func init() {
	rootCommand.AddCommand(applyCmd)

	applyCmd.Flags().StringArrayP("file", "f", []string{""}, "A number of init.yaml plan files")
	applyCmd.Flags().Bool("skip-sealedsecrets", false, "Skip SealedSecrets installation")
	applyCmd.Flags().Bool("skip-create-secrets", false, "Skip creating secrets")
	applyCmd.Flags().Bool("print-plan", false, "Print merged plan and exit")
}

var applyCmd = &cobra.Command{
	Use:          "apply",
	Short:        "Apply configuration for OFC",
	RunE:         runApplyCommandE,
	SilenceUsage: true,
}

type InstallPreferences struct {
	SkipSealedSecrets bool
	SkipCreateSecrets bool
}

func runApplyCommandE(command *cobra.Command, _ []string) error {
	prefs := InstallPreferences{}

	if os.Getuid() == 0 {
		return fmt.Errorf("do not run this tool as root, or on your server. Run it from your own client remotely")
	}

	files, err := command.Flags().GetStringArray("file")
	if err != nil {
		return err
	}
	printPlan, err := command.Flags().GetBool("print-plan")
	if err != nil {
		return err
	}

	prefs.SkipSealedSecrets, err = command.Flags().GetBool("skip-sealedsecrets")
	if err != nil {
		return err
	}
	prefs.SkipCreateSecrets, err = command.Flags().GetBool("skip-create-secrets")
	if err != nil {
		return err
	}

	if len(files) == 0 {
		return fmt.Errorf("provide one or more --file arguments")
	}

	plans := []types.Plan{}
	for _, yamlFile := range files {

		yamlBytes, err := ioutil.ReadFile(yamlFile)
		if err != nil {
			return fmt.Errorf("loading --file %s gave error: %s", yamlFile, err.Error())
		}

		plan := types.Plan{}
		if err := yaml.Unmarshal(yamlBytes, &plan); err != nil {
			return fmt.Errorf("unmarshal of --file %s gave error: %s", yamlFile, err.Error())
		}

		log.Printf("%s loaded\n", yamlFile)
		plans = append(plans, plan)
	}

	log.Printf("Loaded %d plan(s)\n", len(files))
	planMerged, err := types.MergePlans(plans)
	if err != nil {
		return err
	}

	if printPlan {
		out, _ := yaml.Marshal(planMerged)
		fmt.Println(string(out))
		os.Exit(0)
	}

	plan := *planMerged

	plan, err = filterFeatures(plan)
	if err != nil {
		return fmt.Errorf("error while retreiving features: %s", err.Error())
	}

	clientArch, clientOS := env.GetClientArch()
	userDir, err := config.InitUserDir()
	if err != nil {
		return err
	}
	fmt.Printf("User dir: %s\n", userDir)

	install := []string{"kubectl", "helm", "faas-cli", "arkade", "kubeseal"}
	if err := getTools(clientArch, clientOS, userDir, install); err != nil {
		return err
	}

	// pathCurrent := os.Getenv("PATH")
	// newPath := strings.Join(additionalPaths, ":") + ":" + pathCurrent
	// os.Setenv("PATH", newPath)
	// fmt.Printf("Path: %s\n", newPath)

	// To avoid cached versions of tools in /usr/local/bin/
	newPath := "/bin/:/usr/bin/:/usr/sbin/:/sbin/:" + path.Join(userDir, "bin")
	os.Setenv("PATH", newPath)

	log.Printf("Validating tools available in PATH: %q\n", newPath)

	tools := []string{
		"openssl version",
		"kubectl version --client",
		"helm version",
		"faas-cli version",
		"kubeseal --version",
	}

	if err := validateTools(tools); err != nil {
		return errors.Wrap(err, "validateTools")
	}

	if arch := k8s.GetNodeArchitecture(); len(arch) == 0 {
		return fmt.Errorf("unable to detect node architecture. Do not run as root, or directly on a Kubernetes master node")
	}

	if prefs.SkipCreateSecrets == false {
		if err := validatePlan(plan); err != nil {
			return errors.Wrap(err, "validatePlan")
		}
	}

	if err = createNamespaces(); err != nil {
		return errors.Wrap(err, "createNamespaces")
	}

	fmt.Printf("Plan loaded from: %s\n", files)

	os.MkdirAll("tmp", 0700)
	ioutil.WriteFile("tmp/go.mod", []byte("\n"), 0700)

	start := time.Now()
	err = process(plan, prefs)
	done := time.Since(start)

	if err != nil {
		return fmt.Errorf("plan failed after %fs, error: %s", done.Seconds(), err.Error())
	}

	fmt.Printf("Plan completed in %fs.\n", done.Seconds())
	return nil
}

// Vars are variables parsed from flags
type Vars struct {
	YamlFile string
}

func validateTools(tools []string) error {
	for _, tool := range tools {
		err := taskGivesStdout(tool)
		if err != nil {
			return err
		}
	}

	return nil
}

func taskGivesStdout(tool string) error {
	parts := strings.Split(tool, " ")
	args := []string{}

	if len(parts) > 0 {
		args = parts[1:]
	}

	task := execute.ExecTask{
		Command:     parts[0],
		Args:        args,
		StreamStdio: false,
	}

	res, err := task.Execute()
	if err != nil {
		return fmt.Errorf("could not run: '%s', error: %s", tool, err)
	}
	if len(res.Stdout) == 0 {
		return fmt.Errorf("error executing '%s', no output was given - tool is available in PATH", task.Command)
	}
	return nil
}

func validatePlan(plan types.Plan) error {
	for _, secret := range plan.Secrets {
		if featureEnabled(plan.Features, secret.Filters) {
			err := filesExists(secret.Files)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func filesExists(files []types.FileSecret) error {
	if len(files) > 0 {
		for _, file := range files {
			if len(file.ValueCommand) == 0 {
				if _, err := os.Stat(file.ExpandValueFrom()); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func process(plan types.Plan, prefs InstallPreferences) error {

	if plan.OpenFaaSCloudVersion == "" {
		plan.OpenFaaSCloudVersion = "master"
		fmt.Println("No openfaas_cloud_version set in init.yaml, using: master.")
	}

	if err := installIngressController(plan.Ingress); err != nil {
		return errors.Wrap(err, "installIngressController")
	}

	if !prefs.SkipCreateSecrets {
		createSecrets(plan)
	}

	if plan.TLS {
		if err := installCertmanager(); err != nil {
			return errors.Wrap(err, "installCertmanager")
		}
	}

	functionAuthErr := createFunctionsAuth()
	if functionAuthErr != nil {
		log.Println(functionAuthErr.Error())
	}

	if err := installOpenfaas(plan.ScaleToZero, plan.IngressOperator, plan.OpenFaaSOperator); err != nil {
		return errors.Wrap(err, "unable to install openfaas")
	}

	retries := 260
	if plan.TLS {
		for i := 0; i < retries; i++ {
			log.Printf("Is cert-manager ready? %d/%d\n", i+1, retries)
			ready := certManagerReady()
			if ready {
				break
			}
			time.Sleep(time.Second * 2)
		}
	}

	ingressErr := ingress.Apply(plan)
	if ingressErr != nil {
		log.Println(ingressErr)
	}

	if plan.TLS {
		tlsErr := tls.Apply(plan)
		if tlsErr != nil {
			log.Println(tlsErr)
		}
	}

	fmt.Println("Creating stack.yml")

	if err := stack.Apply(plan); err != nil {
		return errors.Wrap(err, "stack.Apply(")
	}

	if !prefs.SkipSealedSecrets {
		if err := installSealedSecrets(); err != nil {
			return errors.Wrap(err, "unable to install sealed-secrets")
		}

		pubCert := exportSealedSecretPubCert()
		writeErr := ioutil.WriteFile("tmp/pubcert.pem", []byte(pubCert), 0700)
		if writeErr != nil {
			log.Println(writeErr)
			return writeErr
		}
	}

	if err := cloneCloudComponents(plan.OpenFaaSCloudVersion); err != nil {
		return errors.Wrap(err, "cloneCloudComponents")
	}

	if err := deployCloudComponents(plan); err != nil {
		return errors.Wrap(err, "deployCloudComponents")
	}

	return nil
}

func helmRepoAdd(name, repo string) error {
	log.Printf("Adding %s helm repo\n", name)

	task := execute.ExecTask{
		Command:     "helm",
		Args:        []string{"repo", "add", name, repo},
		StreamStdio: false,
	}

	taskRes, taskErr := task.Execute()

	if taskErr != nil {
		return taskErr
	}

	if len(taskRes.Stderr) > 0 {
		log.Println(taskRes.Stderr)
	}

	return nil
}

func helmRepoAddStable() error {
	log.Println("Adding stable helm repo")

	task := execute.ExecTask{
		Command:     "helm",
		StreamStdio: false,
	}

	taskRes, taskErr := task.Execute()

	if taskErr != nil {
		return taskErr
	}

	if len(taskRes.Stderr) > 0 {
		log.Println(taskRes.Stderr)
	}

	return nil
}

func helmRepoUpdate() error {
	log.Println("Updating helm repos")

	task := execute.ExecTask{
		Command:     "helm",
		Args:        []string{"repo", "update"},
		StreamStdio: false,
	}

	taskRes, taskErr := task.Execute()

	if taskErr != nil {
		return taskErr
	}

	if len(taskRes.Stderr) > 0 {
		log.Println(taskRes.Stderr)
	}

	return nil
}

func createFunctionsAuth() error {
	log.Println("Creating secrets for functions to consume")

	task := execute.ExecTask{
		Command:     "scripts/create-functions-auth.sh",
		Shell:       true,
		StreamStdio: false,
	}

	taskRes, err := task.Execute()

	if err != nil {
		return err
	}

	if len(taskRes.Stderr) > 0 {
		log.Println(taskRes.Stderr)
	}

	return nil
}

func installIngressController(ingress string) error {
	log.Println("Installing ingress-nginx")

	env := []string{"PATH=" + os.Getenv("PATH")}

	// Adding wait took quite a long time, so disabling that.
	args := []string{"install", "ingress-nginx"}
	if ingress == "host" {
		args = append(args, "--host-mode")
	}

	task := execute.ExecTask{
		Command:     "arkade",
		Args:        args,
		Shell:       true,
		Env:         env,
		StreamStdio: false,
	}

	res, err := task.Execute()
	if err != nil {
		return errors.Wrap(err, "error installing ingress-nginx")
	}

	if res.ExitCode != 0 {
		return fmt.Errorf("non-zero exit-code: %s %s", res.Stdout, res.Stderr)
	}

	if len(res.Stderr) > 0 {
		log.Printf("stderr: %s\n", res.Stderr)
	}
	return nil
}

func installSealedSecrets() error {
	log.Println("Installing sealed-secrets")

	var env []string
	args := []string{"install", "sealed-secrets", "--namespace=kube-system", "--wait"}

	task := execute.ExecTask{
		Command:     "arkade",
		Args:        args,
		Shell:       true,
		Env:         env,
		StreamStdio: false,
	}

	res, err := task.Execute()
	if err != nil {
		return err
	}

	if res.ExitCode != 0 {
		return fmt.Errorf("non-zero exit-code: %s %s", res.Stdout, res.Stderr)
	}

	if len(res.Stderr) > 0 {
		log.Printf("stderr: %s\n", res.Stderr)
	}
	return nil
}

func installOpenfaas(scaleToZero, ingressOperator, openfaasOperator bool) error {
	log.Println("Installing openfaas")

	args := []string{"install", "openfaas",
		"--set basic_auth=true",
		"--set functionNamespace=openfaas-fn",
		"--set ingress.enabled=false",
		"--set gateway.scaleFromZero=true",
		"--set gateway.readTimeout=15m",
		"--set gateway.writeTimeout=15m",
		"--set gateway.upstreamTimeout=14m55s",
		"--set queueWorker.ackWait=15m",
		"--set faasnetes.readTimeout=5m",
		"--set faasnetes.writeTimeout=5m",
		"--set gateway.replicas=2",
		"--set queueWorker.replicas=2",
		"--set faasIdler.dryRun=" + strconv.FormatBool(!scaleToZero),
		"--set faasnetes.httpProbe=true",
		"--set faasnetes.imagePullPolicy=IfNotPresent",
		"--set ingressOperator.create=" + strconv.FormatBool(ingressOperator),
		"--set operator.create=" + strconv.FormatBool(openfaasOperator),
		"--wait",
	}

	task := execute.ExecTask{
		Command:     "arkade",
		Args:        args,
		Shell:       true,
		StreamStdio: false,
	}

	res, err := task.Execute()
	if err != nil {
		return err
	}

	if res.ExitCode != 0 {
		return fmt.Errorf("non-zero exit-code: %s %s", res.Stdout, res.Stderr)
	}

	if len(res.Stderr) > 0 {
		log.Printf("stderr: %s\n", res.Stderr)
	}

	return nil
}

func installCertmanager() error {
	log.Println("Installing cert-manager")

	args := []string{"install", "cert-manager", "--wait"}
	task := execute.ExecTask{
		Command:     "arkade",
		Args:        args,
		Shell:       true,
		StreamStdio: false,
	}

	res, err := task.Execute()
	if err != nil {
		return err
	}

	if res.ExitCode != 0 {
		return fmt.Errorf("non-zero exit-code: %s %s", res.Stdout, res.Stderr)
	}

	if len(res.Stderr) > 0 {
		log.Printf("stderr: %s\n", res.Stderr)
	}
	return nil
}

func createSecrets(plan types.Plan) error {
	for _, secret := range plan.Secrets {
		if featureEnabled(plan.Features, secret.Filters) {
			fmt.Printf("Creating secret: %s\n", secret.Name)

			command := types.BuildSecretTask(secret)
			fmt.Printf("Secret - %s %s\n", command.Command, strings.Join(command.Args, " "))
			res, err := command.Execute()
			if err != nil {
				log.Println(err)
			}

			out := res.Stdout
			if len(res.Stderr) > 0 {
				out = out + " / " + res.Stderr
			}
			fmt.Printf("%s\n", out)
		}
	}

	return nil
}

func sealedSecretsReady() bool {

	task := execute.ExecTask{
		Command:     "./scripts/get-sealedsecretscontroller.sh",
		Shell:       true,
		StreamStdio: false,
	}

	res, err := task.Execute()
	fmt.Println("sealedsecretscontroller", res.ExitCode, res.Stdout, res.Stderr, err)
	return res.Stdout == "1"
}

func exportSealedSecretPubCert() string {

	task := execute.ExecTask{
		Command:     "./scripts/export-sealed-secret-pubcert.sh",
		Shell:       true,
		StreamStdio: false,
		Env:         []string{"PATH=" + os.Getenv("PATH")},
	}

	res, err := task.Execute()
	fmt.Println("secrets cert", res.ExitCode, res.Stdout, res.Stderr, err)
	return res.Stdout
}

func certManagerReady() bool {
	task := execute.ExecTask{
		Command:     "./scripts/get-cert-manager.sh",
		Shell:       true,
		StreamStdio: false,
	}

	res, err := task.Execute()
	fmt.Println("cert-manager", res.ExitCode, res.Stdout, res.Stderr, err)
	return res.Stdout == "True"
}

func cloneCloudComponents(tag string) error {
	task := execute.ExecTask{
		Command: "./scripts/clone-cloud-components.sh",
		Shell:   true,
		Env: []string{
			fmt.Sprintf("TAG=%v", tag),
		},
		StreamStdio: false,
	}

	res, err := task.Execute()
	if err != nil {
		return err
	}

	fmt.Println(res)

	return nil
}

func deployCloudComponents(plan types.Plan) error {

	gitlabEnv := ""
	if plan.SCM == "gitlab" {
		gitlabEnv = "GITLAB=true"
	}

	networkPoliciesEnv := ""
	if plan.NetworkPolicies {
		networkPoliciesEnv = "ENABLE_NETWORK_POLICIES=true"
	}

	task := execute.ExecTask{
		Command: "./scripts/deploy-cloud-components.sh",
		Shell:   true,
		Env: []string{
			gitlabEnv,
			networkPoliciesEnv,
		},
		StreamStdio: false,
	}

	res, err := task.Execute()
	if err != nil {
		return err
	}

	fmt.Println(res)

	return nil
}

func featureEnabled(features []string, secretFeatures []string) bool {
	for _, feature := range features {
		for _, secretFeature := range secretFeatures {
			if feature == secretFeature {
				return true
			}
		}
	}
	return false
}

func filterFeatures(plan types.Plan) (types.Plan, error) {
	var err error

	plan.Features = append(plan.Features, types.DefaultFeature)

	plan, err = filterGitRepositoryManager(plan)
	if err != nil {
		return plan, fmt.Errorf("Error while filtering features: %s", err.Error())
	}

	if plan.TLS == true {
		plan, err = filterDNSFeature(plan)
		if err != nil {
			return plan, fmt.Errorf("Error while filtering features: %s", err.Error())
		}
	}

	return plan, err
}

func filterDNSFeature(plan types.Plan) (types.Plan, error) {
	if plan.TLSConfig.DNSService == types.DigitalOcean {
		plan.Features = append(plan.Features, types.DODNS)
	} else if plan.TLSConfig.DNSService == types.CloudDNS {
		plan.Features = append(plan.Features, types.GCPDNS)
	} else if plan.TLSConfig.DNSService == types.Route53 {
		plan.Features = append(plan.Features, types.Route53DNS)
	} else if plan.TLSConfig.DNSService == types.Cloudflare {
		plan.Features = append(plan.Features, types.CloudflareDNS)
	} else {
		return plan, fmt.Errorf("Error unavailable DNS service provider: %s", plan.TLSConfig.DNSService)
	}
	return plan, nil
}

func filterGitRepositoryManager(plan types.Plan) (types.Plan, error) {
	if plan.SCM == types.GitLabSCM {
		plan.Features = append(plan.Features, types.GitLabFeature)
	} else if plan.SCM == types.GitHubSCM {
		plan.Features = append(plan.Features, types.GitHubFeature)
	} else {
		return plan, fmt.Errorf("Error unsupported Git repository manager: %s", plan.SCM)
	}
	return plan, nil
}

func getTools(clientArch, clientOS, userDir string, install []string) error {
	tools := get.MakeTools()
	displayProgess := true
	for _, t := range install {
		if tool, err := getTool(t, tools); tool != nil {
			filePath := path.Join(path.Join(userDir, "bin"), tool.Name)
			if _, err := os.Stat(filePath); err != nil {
				_, finalName, err := get.Download(tool, clientArch, clientOS, tool.Version, get.DownloadArkadeDir, displayProgess)
				if err != nil {
					return err
				}
				fmt.Printf("Downloaded tool: %s\n", finalName)
			} else {
				fmt.Printf("Skipping tool: %s\n", tool.Name)
			}
		} else {
			return err
		}
	}
	return nil
}

func getTool(name string, tools []get.Tool) (*get.Tool, error) {
	var tool *get.Tool
	for _, t := range tools {
		if t.Name == name {
			tool = &t
			break
		}
	}
	if tool == nil {
		return nil, fmt.Errorf("unable to find tool definition")
	}

	return tool, nil
}

// createNamespaces is required for secrets to be created
// before each app is installed. Including: cert-manager for TLS
// secrets and openfaas/openfaas-fn for function secrets.
func createNamespaces() error {
	res, err := k8s.KubectlTask("apply", "-f", "https://raw.githubusercontent.com/openfaas/faas-netes/master/namespaces.yml")
	if err != nil {
		return err
	}
	if res.ExitCode != 0 {
		return fmt.Errorf("error creating openfaas namespaces: %s %s", res.Stdout, res.Stderr)
	}
	fmt.Printf("Applied namespaces for openfaas\n")

	ns := `apiVersion: v1
kind: Namespace
metadata:
  creationTimestamp: null
  name: cert-manager
spec: {}
status: {}
`
	buffer := bytes.NewReader([]byte(ns))
	res, err = k8s.KubectlTaskStdin(buffer, "apply", "-f", "-")
	if err != nil {
		return err
	}
	if res.ExitCode != 0 {
		return fmt.Errorf("error creating openfaas namespaces: %s %s", res.Stdout, res.Stderr)
	}
	fmt.Printf("Applied namespaces for cert-manager\n")

	return nil
}
