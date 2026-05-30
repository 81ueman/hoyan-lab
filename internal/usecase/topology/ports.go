package topology

import (
	"github.com/81ueman/hoyan-lab/internal/adapter/configparse"
	"github.com/81ueman/hoyan-lab/internal/adapter/labfile"
	"github.com/81ueman/hoyan-lab/internal/domain/model"
)

type LabFileLoader interface {
	Load(path string) (labfile.File, error)
}

type ConfigParser interface {
	Parse(kind model.DeviceKind, path string, opts configparse.ParseOptions) (configparse.ParseResult, error)
	ParseNftablesACL(path string) ([]model.ACL, []model.ACLBinding, error)
}

type Builder struct {
	loader LabFileLoader
	parser ConfigParser
}

func NewBuilder(loader LabFileLoader, parser ConfigParser) Builder {
	return Builder{loader: loader, parser: parser}
}

type labFileLoader struct{}

func (labFileLoader) Load(path string) (labfile.File, error) {
	return labfile.Load(path)
}

type configParser struct{}

func (configParser) Parse(kind model.DeviceKind, path string, opts configparse.ParseOptions) (configparse.ParseResult, error) {
	return configparse.ParseConfigWithOptions(kind, path, opts)
}

func (configParser) ParseNftablesACL(path string) ([]model.ACL, []model.ACLBinding, error) {
	return configparse.ParseNftablesACLConfig(path)
}

func defaultBuilder() Builder {
	return NewBuilder(labFileLoader{}, configParser{})
}
