package snapshot

import (
	"errors"
	"fmt"
	"io"

	"gopkg.in/yaml.v3"
)

// Decode は未知のフィールドや複数文書を拒否して manifest を読み込みます。
func Decode(reader io.Reader) (*Manifest, error) {
	decoder := yaml.NewDecoder(reader)
	decoder.KnownFields(true)

	var manifest Manifest
	if err := decoder.Decode(&manifest); err != nil {
		return nil, fmt.Errorf("manifest の YAML 形式またはフィールドが不正です")
	}

	if manifest.Tools == nil {
		return nil, fmt.Errorf("manifest の tools はマネージャ別の一覧を指定してください")
	}

	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("manifest は単一の YAML 文書で指定してください")
	}

	if manifest.Version != 1 {
		return nil, fmt.Errorf("manifest のスキーマバージョンは 1 が必要です")
	}

	for manager, tools := range manifest.Tools {
		items, err := Normalize(manager, tools)
		if err != nil {
			return nil, err
		}

		if tools == nil {
			return nil, fmt.Errorf("%s: ツールは配列で指定してください（空一覧は []）", manager)
		}

		manifest.Tools[manager] = items
	}

	return &manifest, nil
}
