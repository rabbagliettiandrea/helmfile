package state

import (
	"fmt"
	"path/filepath"

	"github.com/imdario/mergo"
	"go.uber.org/zap"

	"github.com/helmfile/helmfile/pkg/environment"
	"github.com/helmfile/helmfile/pkg/filesystem"
	"github.com/helmfile/helmfile/pkg/maputil"
	"github.com/helmfile/helmfile/pkg/remote"
	"github.com/helmfile/helmfile/pkg/tmpl"
	"github.com/helmfile/helmfile/pkg/yaml"
)

type EnvironmentValuesLoader struct {
	storage *Storage

	fs *filesystem.FileSystem

	logger *zap.SugaredLogger

	remote *remote.Remote
}

func NewEnvironmentValuesLoader(storage *Storage, fs *filesystem.FileSystem, logger *zap.SugaredLogger, remote *remote.Remote) *EnvironmentValuesLoader {
	return &EnvironmentValuesLoader{
		storage: storage,
		fs:      fs,
		logger:  logger,
		remote:  remote,
	}
}

func (ld *EnvironmentValuesLoader) LoadEnvironmentValues(missingFileHandler *string, valuesEntries []interface{}, ctxEnv *environment.Environment, envName string) (map[string]interface{}, error) {
	result := map[string]interface{}{}

	for _, entry := range valuesEntries {
		maps := []interface{}{}

		switch strOrMap := entry.(type) {
		case string:
			files, skipped, err := ld.storage.resolveFile(missingFileHandler, "environment values", entry.(string))
			if err != nil {
				return nil, err
			}
			if skipped {
				continue
			}

			for _, f := range files {
				var env environment.Environment
				if ctxEnv == nil {
					env = *environment.New(envName)
				} else {
					env = *ctxEnv
				}

				tmplData := NewEnvironmentTemplateData(env, "", map[string]interface{}{})
				r := tmpl.NewFileRenderer(ld.fs, filepath.Dir(f), tmplData)
				bytes, err := r.RenderToBytes(f)
				if err != nil {
					return nil, fmt.Errorf("failed to load environment values file \"%s\": %v", f, err)
				}
				m := map[string]interface{}{}
				if err := yaml.Unmarshal(bytes, &m); err != nil {
					return nil, fmt.Errorf("failed to load environment values file \"%s\": %v\n\nOffending YAML:\n%s", f, err, bytes)
				}
				maps = append(maps, m)
				ld.logger.Debugf("envvals_loader: loaded %s:%v", strOrMap, m)
			}
		case map[interface{}]interface{}, map[string]interface{}:
			maps = append(maps, strOrMap)
		default:
			return nil, fmt.Errorf("unexpected type of value: value=%v, type=%T", strOrMap, strOrMap)
		}
		for _, m := range maps {
			// All the nested map key should be string. Otherwise we get strange errors due to that
			// mergo or reflect is unable to merge map[interface{}]interface{} with map[string]interface{} or vice versa.
			// See https://github.com/roboll/helmfile/issues/677
			vals, err := maputil.CastKeysToStrings(m)
			if err != nil {
				return nil, err
			}
			if err := mergo.Merge(&result, &vals, mergo.WithOverride); err != nil {
				return nil, fmt.Errorf("failed to merge %v: %v", m, err)
			}
		}
	}

	return result, nil
}
