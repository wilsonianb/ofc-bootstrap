package stack

import (
	"bytes"
	"html/template"
	"io/ioutil"
	"os"

	"github.com/openfaas/ofc-bootstrap/pkg/types"
)

type gatewayConfig struct {
	RootDomain       string
	Scheme           string
	CustomTemplates  string
	InvocationCost   string
	InvocationUnits  string
	BonusInvocations string
}

type authConfig struct {
}

type stackConfig struct {
}

// Apply creates `templates/gateway_config.yml` to be referenced by stack.yml
func Apply(plan types.Plan) error {
	scheme := "http"
	if plan.TLS {
		scheme += "s"
	}

	if gwConfigErr := generateTemplate("gateway_config", plan, gatewayConfig{
		RootDomain:       plan.RootDomain,
		Scheme:           scheme,
		CustomTemplates:  plan.Deployment.FormatCustomTemplates(),
		InvocationCost:   plan.Pricing.InvocationCost,
		InvocationUnits:  plan.Pricing.InvocationUnits,
		BonusInvocations: plan.Pricing.BonusInvocations,
	}); gwConfigErr != nil {
		return gwConfigErr
	}

	if ofAuthDepErr := generateTemplate("edge-auth-dep", plan, authConfig{}); ofAuthDepErr != nil {
		return ofAuthDepErr
	}

	if stackErr := generateTemplate("stack", plan, stackConfig{}); stackErr != nil {
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
