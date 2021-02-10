package stack

import (
	"bytes"
	"html/template"
	"io/ioutil"
	"os"

	"github.com/openfaas/ofc-bootstrap/pkg/types"
)

type gitlabConfig struct {
	GitLabInstance      string `yaml:"gitlab_instance,omitempty"`
	CustomersSecretPath string
}

type gatewayConfig struct {
	RootDomain           string
	CustomersURL         string
	Scheme               string
	CustomTemplates      string
	EnableDockerfileLang bool
	BuildBranch          string
	CustomersSecretPath  string
}

type authConfig struct {
	RootDomain            string
	ClientId              string
	CustomersURL          string
	Scheme                string
	OAuthProvider         string
	OAuthProviderBaseURL  string
	OFCustomersSecretPath string
	TLSEnabled            bool
}

type stackConfig struct {
	GitHub              bool
	CustomersSecretPath string
}

type dashboardConfig struct {
	RootDomain     string
	Scheme         string
	GitHubAppUrl   string
	GitLabInstance string
}

// Apply creates `templates/gateway_config.yml` to be referenced by stack.yml
func Apply(plan types.Plan) error {
	scheme := "http"
	if plan.TLS {
		scheme += "s"
	}

	customersSecretPath := ""

	if plan.CustomersSecret {
		customersSecretPath = "/var/openfaas/secrets/customers"
	}

	if gwConfigErr := generateTemplate("gateway_config", plan, gatewayConfig{
		RootDomain:           plan.RootDomain,
		CustomersURL:         plan.CustomersURL,
		Scheme:               scheme,
		CustomTemplates:      plan.Deployment.FormatCustomTemplates(),
		EnableDockerfileLang: plan.EnableDockerfileLang,
		BuildBranch:          plan.BuildBranch,
	}); gwConfigErr != nil {
		return gwConfigErr
	}

	if githubConfigErr := generateTemplate("github", plan, types.Github{
		AppID:          plan.Github.AppID,
		PrivateKeyFile: plan.Github.PrivateKeyFile,
	}); githubConfigErr != nil {
		return githubConfigErr
	}

	if plan.SCM == "gitlab" {
		if gitlabConfigErr := generateTemplate("gitlab", plan, gitlabConfig{
			GitLabInstance:      plan.Gitlab.GitLabInstance,
			CustomersSecretPath: customersSecretPath,
		}); gitlabConfigErr != nil {
			return gitlabConfigErr
		}
	}

	var gitHubAppUrl, gitLabInstance string
	if plan.SCM == types.GitHubSCM {
		gitHubAppUrl = plan.Github.PublicLink
	} else if plan.SCM == types.GitLabSCM {
		gitLabInstance = plan.Gitlab.GitLabInstance
	}
	dashboardConfigErr := generateTemplate("dashboard_config", plan, dashboardConfig{
		RootDomain:     plan.RootDomain,
		Scheme:         scheme,
		GitHubAppUrl:   gitHubAppUrl,
		GitLabInstance: gitLabInstance,
	})
	if dashboardConfigErr != nil {
		return dashboardConfigErr
	}

	if plan.EnableOAuth {
		ofCustomersSecretPath := ""
		if plan.CustomersSecret {
			ofCustomersSecretPath = "/var/secrets/of-customers/of-customers"
		}

		if ofAuthDepErr := generateTemplate("edge-auth-dep", plan, authConfig{
			RootDomain:            plan.RootDomain,
			ClientId:              plan.OAuth.ClientId,
			CustomersURL:          plan.CustomersURL,
			Scheme:                scheme,
			OAuthProvider:         plan.SCM,
			OAuthProviderBaseURL:  plan.OAuth.OAuthProviderBaseURL,
			OFCustomersSecretPath: ofCustomersSecretPath,
			TLSEnabled:            plan.TLS,
		}); ofAuthDepErr != nil {
			return ofAuthDepErr
		}
	}

	isGitHub := plan.SCM == "github"
	if stackErr := generateTemplate("stack", plan, stackConfig{
		GitHub:              isGitHub,
		CustomersSecretPath: customersSecretPath,
	}); stackErr != nil {
		return stackErr
	}

	return nil
}

func generateTemplate(fileName string, plan types.Plan, templateType interface{}) error {

	generatedData, err := applyTemplate("templates/"+fileName+".yml", templateType)
	if err != nil {
		return err
	}

	tempFilePath := "tmp/generated-" + fileName + ".yml"
	file, fileErr := os.Create(tempFilePath)
	if fileErr != nil {
		return fileErr
	}
	defer file.Close()

	_, writeErr := file.Write(generatedData)
	file.Close()

	if writeErr != nil {
		return writeErr
	}

	return nil
}

func applyTemplate(templateFileName string, templateType interface{}) ([]byte, error) {
	data, err := ioutil.ReadFile(templateFileName)
	if err != nil {
		return nil, err
	}
	t := template.Must(template.New(templateFileName).Parse(string(data)))

	buffer := new(bytes.Buffer)

	executeErr := t.Execute(buffer, templateType)

	return buffer.Bytes(), executeErr
}
