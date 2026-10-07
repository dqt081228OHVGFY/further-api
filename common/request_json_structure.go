package common

import (
	"context"
	"fmt"
	"io"

	"github.com/dqt081228OHVGFY/further-api/constant"
)

// A streaming lexical bound precedes Go/JavaScript object allocation. Strings
// are counted without decoding or buffering their contents; the ordinary JSON
// decoder remains responsible for syntax, UTF-8 and destination types.
func ValidateRequestJSONStructure(ctx context.Context, storage BodyStorage) error {
	reader, err := storage.NewReader()
	if err != nil {
		return err
	}
	defer reader.Close()
	maxDepth, maxNodes := constant.RequestJSONMaxDepth, constant.RequestJSONMaxNodes
	if maxDepth <= 0 {
		maxDepth = 128
	}
	if maxNodes <= 0 {
		maxNodes = 100000
	}
	buffer := make([]byte, 32<<10)
	depth, nodes := 0, int64(0)
	started, inString, escaped, primitive := false, false, false, false
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		n, readErr := reader.Read(buffer)
		for _, character := range buffer[:n] {
			if !started {
				if character == ' ' || character == '\t' || character == '\n' || character == '\r' {
					continue
				}
				if character != '{' && character != '[' {
					return nil
				}
				started = true
			}
			if inString {
				if escaped {
					escaped = false
					continue
				}
				if character == '\\' {
					escaped = true
					continue
				}
				if character == '"' {
					inString = false
				}
				continue
			}
			switch character {
			case '"':
				inString = true
				primitive = false
				nodes++
			case '{', '[':
				depth++
				primitive = false
				nodes++
			case '}', ']':
				depth--
				primitive = false
			case ':', ',', ' ', '\t', '\r', '\n':
				primitive = false
			default:
				if !primitive {
					nodes++
					primitive = true
				}
			}
			if depth > maxDepth || nodes > int64(maxNodes) {
				return fmt.Errorf("%w JSON结构超过配置的深度/节点上限。", ErrRequestBodyTooLarge)
			}
		}
		if readErr != nil {
			if readErr == io.EOF {
				break
			}
			return readErr
		}
	}
	if bounded, ok := storage.(*budgetedBodyStorage); ok {
		return bounded.reserveJSONNodes(nodes)
	}
	return nil
}
