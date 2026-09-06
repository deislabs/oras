/*
Copyright The ORAS Authors.
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package display

import (
	"io"
	"os"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	fetcher "github.com/oras-project/oras-go/v3/content"

	"oras.land/oras/cmd/oras/internal/display/content"
	"oras.land/oras/cmd/oras/internal/display/metadata"
	"oras.land/oras/cmd/oras/internal/display/metadata/descriptor"
	"oras.land/oras/cmd/oras/internal/display/metadata/json"
	"oras.land/oras/cmd/oras/internal/display/metadata/table"
	"oras.land/oras/cmd/oras/internal/display/metadata/template"
	"oras.land/oras/cmd/oras/internal/display/metadata/text"
	"oras.land/oras/cmd/oras/internal/display/metadata/tree"
	"oras.land/oras/cmd/oras/internal/display/status"
	"oras.land/oras/cmd/oras/internal/errors"
	"oras.land/oras/cmd/oras/internal/option"
	"oras.land/oras/cmd/oras/internal/output"
)

// NewPushHandler returns status and metadata handlers for push command.
func NewPushHandler(printer *output.Printer, format option.Format, tty *os.File, fetcher fetcher.Fetcher) (status.PushHandler, metadata.PushHandler, error) {
	var statusHandler status.PushHandler
	if tty != nil {
		statusHandler = status.NewTTYPushHandler(tty, fetcher)
	} else if format.Type == option.FormatTypeText.Name {
		statusHandler = status.NewTextPushHandler(printer, fetcher)
	} else {
		statusHandler = status.NewDiscardHandler()
	}

	var metadataHandler metadata.PushHandler
	switch format.Type {
	case option.FormatTypeText.Name:
		metadataHandler = text.NewPushHandler(printer)
	case option.FormatTypeJSON.Name:
		metadataHandler = json.NewPushHandler(printer)
	case option.FormatTypeGoTemplate.Name:
		metadataHandler = template.NewPushHandler(printer, format.Template)
	default:
		return nil, nil, errors.UnsupportedFormatTypeError(format.Type)
	}
	return statusHandler, metadataHandler, nil
}

// NewAttachHandler returns status and metadata handlers for attach command.
func NewAttachHandler(printer *output.Printer, format option.Format, tty *os.File, fetcher fetcher.Fetcher) (status.AttachHandler, metadata.AttachHandler, error) {
	var statusHandler status.AttachHandler
	if tty != nil {
		statusHandler = status.NewTTYAttachHandler(tty, fetcher)
	} else if format.Type == option.FormatTypeText.Name {
		statusHandler = status.NewTextAttachHandler(printer, fetcher)
	} else {
		statusHandler = status.NewDiscardHandler()
	}

	var metadataHandler metadata.AttachHandler
	switch format.Type {
	case option.FormatTypeText.Name:
		metadataHandler = text.NewAttachHandler(printer)
	case option.FormatTypeJSON.Name:
		metadataHandler = json.NewAttachHandler(printer)
	case option.FormatTypeGoTemplate.Name:
		metadataHandler = template.NewAttachHandler(printer, format.Template)
	default:
		return nil, nil, errors.UnsupportedFormatTypeError(format.Type)
	}
	return statusHandler, metadataHandler, nil
}

// NewPullHandler returns status and metadata handlers for pull command.
func NewPullHandler(printer *output.Printer, format option.Format, path string, tty *os.File) (status.PullHandler, metadata.PullHandler, error) {
	var statusHandler status.PullHandler
	if tty != nil {
		statusHandler = status.NewTTYPullHandler(tty)
	} else if format.Type == option.FormatTypeText.Name {
		statusHandler = status.NewTextPullHandler(printer)
	} else {
		statusHandler = status.NewDiscardHandler()
	}

	var metadataHandler metadata.PullHandler
	switch format.Type {
	case option.FormatTypeText.Name:
		metadataHandler = text.NewPullHandler(printer)
	case option.FormatTypeJSON.Name:
		metadataHandler = json.NewPullHandler(printer, path)
	case option.FormatTypeGoTemplate.Name:
		metadataHandler = template.NewPullHandler(printer, path, format.Template)
	default:
		return nil, nil, errors.UnsupportedFormatTypeError(format.Type)
	}
	return statusHandler, metadataHandler, nil
}

// NewDiscoverHandler returns status and metadata handlers for discover command.
func NewDiscoverHandler(out io.Writer, format option.Format, path string, rawReference string, desc ocispec.Descriptor, verbose bool, tty *os.File) (metadata.DiscoverHandler, error) {
	var handler metadata.DiscoverHandler
	switch format.Type {
	case option.FormatTypeTree.Name:
		handler = tree.NewDiscoverHandler(out, path, desc, verbose, tty)
	case option.FormatTypeTable.Name:
		handler = table.NewDiscoverHandler(out, rawReference, desc, verbose)
	case option.FormatTypeJSON.Name:
		handler = json.NewDiscoverHandler(out, desc, path)
	case option.FormatTypeGoTemplate.Name:
		handler = template.NewDiscoverHandler(out, desc, path, format.Template)
	default:
		return nil, errors.UnsupportedFormatTypeError(format.Type)
	}
	return handler, nil
}

// NewManifestFetchHandler returns a manifest fetch handler.
func NewManifestFetchHandler(out io.Writer, format option.Format, outputDescriptor, pretty bool, outputPath string) (metadata.ManifestFetchHandler, content.ManifestFetchHandler, error) {
	var metadataHandler metadata.ManifestFetchHandler
	var contentHandler content.ManifestFetchHandler

	switch format.Type {
	case option.FormatTypeText.Name:
		// raw
		if outputDescriptor {
			metadataHandler = descriptor.NewManifestFetchHandler(out, pretty)
		} else {
			metadataHandler = metadata.NewDiscardHandler()
		}
	case option.FormatTypeJSON.Name:
		// json
		metadataHandler = json.NewManifestFetchHandler(out)
		if outputPath == "" {
			contentHandler = content.NewDiscardHandler()
		}
	case option.FormatTypeGoTemplate.Name:
		// go template
		metadataHandler = template.NewManifestFetchHandler(out, format.Template)
		if outputPath == "" {
			contentHandler = content.NewDiscardHandler()
		}
	default:
		return nil, nil, errors.UnsupportedFormatTypeError(format.Type)
	}

	if contentHandler == nil {
		contentHandler = content.NewManifestFetchHandler(out, pretty, outputPath)
	}
	return metadataHandler, contentHandler, nil
}

// NewTagHandler returns a tag handler.
func NewTagHandler(printer *output.Printer, target option.Target) metadata.TagHandler {
	return text.NewTagHandler(printer, target)
}

// NewManifestPushHandler returns a manifest push handler.
func NewManifestPushHandler(printer *output.Printer, outputDescriptor bool, _ bool, desc ocispec.Descriptor, target *option.Target) (status.ManifestPushHandler, metadata.ManifestPushHandler) {
	if outputDescriptor {
		return status.NewDiscardHandler(), metadata.NewDiscardHandler()
	}
	return status.NewTextManifestPushHandler(printer, desc), text.NewManifestPushHandler(printer, target)
}

// NewManifestDeleteHandler returns a manifest delete handler.
func NewManifestDeleteHandler(printer *output.Printer, target *option.Target) metadata.ManifestDeleteHandler {
	return text.NewManifestDeleteHandler(printer, target)
}

// NewManifestIndexCreateHandler returns status, metadata and content handlers for index create command.
func NewManifestIndexCreateHandler(outputPath string, printer *output.Printer, pretty bool) (status.ManifestIndexCreateHandler, metadata.ManifestIndexCreateHandler, content.ManifestIndexCreateHandler) {
	var statusHandler status.ManifestIndexCreateHandler
	var metadataHandler metadata.ManifestIndexCreateHandler
	var contentHandler content.ManifestIndexCreateHandler
	switch outputPath {
	case "":
		statusHandler = status.NewTextManifestIndexCreateHandler(printer)
		metadataHandler = text.NewManifestIndexCreateHandler(printer)
		contentHandler = content.NewDiscardHandler()
	case "-":
		statusHandler = status.NewDiscardHandler()
		metadataHandler = metadata.NewDiscardHandler()
		contentHandler = content.NewManifestIndexCreateHandler(printer, pretty, outputPath)
	default:
		statusHandler = status.NewTextManifestIndexCreateHandler(printer)
		metadataHandler = text.NewManifestIndexCreateHandler(printer)
		contentHandler = content.NewManifestIndexCreateHandler(printer, pretty, outputPath)
	}
	return statusHandler, metadataHandler, contentHandler
}

// NewManifestIndexUpdateHandler returns status, metadata and content handlers for index update command.
func NewManifestIndexUpdateHandler(outputPath string, printer *output.Printer, pretty bool) (
	status.ManifestIndexUpdateHandler,
	metadata.ManifestIndexUpdateHandler,
	content.ManifestIndexUpdateHandler) {
	statusHandler := status.NewTextManifestIndexUpdateHandler(printer)
	metadataHandler := text.NewManifestIndexCreateHandler(printer)
	contentHandler := content.NewManifestIndexCreateHandler(printer, pretty, outputPath)
	switch outputPath {
	case "":
		contentHandler = content.NewDiscardHandler()
	case "-":
		statusHandler = status.NewDiscardHandler()
		metadataHandler = metadata.NewDiscardHandler()
	}
	return statusHandler, metadataHandler, contentHandler
}

// NewCopyHandler returns copy handlers.
func NewCopyHandler(printer *output.Printer, tty *os.File, fetcher fetcher.Fetcher) (status.CopyHandler, metadata.CopyHandler) {
	if tty != nil {
		return status.NewTTYCopyHandler(tty), text.NewCopyHandler(printer)
	}
	return status.NewTextCopyHandler(printer, fetcher), text.NewCopyHandler(printer)
}

// NewBackupHandler returns backup handlers.
func NewBackupHandler(printer *output.Printer, tty *os.File, repo string, fetcher fetcher.Fetcher) (status.BackupHandler, metadata.BackupHandler) {
	if tty != nil {
		return status.NewTTYBackupHandler(tty, fetcher), text.NewBackupHandler(repo, printer)
	}
	return status.NewTextBackupHandler(printer, fetcher), text.NewBackupHandler(repo, printer)
}

// NewRestoreHandler returns restore handlers.
func NewRestoreHandler(printer *output.Printer, tty *os.File, fetcher fetcher.Fetcher, dryRun bool) (status.RestoreHandler, metadata.RestoreHandler) {
	if tty != nil {
		return status.NewTTYRestoreHandler(tty, fetcher), text.NewRestoreHandler(printer, dryRun)
	}
	return status.NewTextRestoreHandler(printer, fetcher), text.NewRestoreHandler(printer, dryRun)
}

// NewBlobPushHandler returns blob push handlers.
func NewBlobPushHandler(printer *output.Printer, outputDescriptor bool, _ bool, desc ocispec.Descriptor, tty *os.File) (status.BlobPushHandler, metadata.BlobPushHandler) {
	if outputDescriptor {
		return status.NewDiscardHandler(), metadata.NewDiscardHandler()
	}
	if tty != nil {
		return status.NewTTYBlobPushHandler(tty, desc), text.NewBlobPushHandler(printer, desc)
	}
	return status.NewTextBlobPushHandler(printer, desc), text.NewBlobPushHandler(printer, desc)
}

// NewResolveHandler returns a resolve metadata handler.
func NewResolveHandler(printer *output.Printer, fullRef bool, path string) metadata.ResolveHandler {
	return text.NewResolveHandler(printer, fullRef, path)
}

// NewBlobDeleteHandler returns blob delete handlers.
func NewBlobDeleteHandler(printer *output.Printer, target *option.Target) metadata.BlobDeleteHandler {
	return text.NewBlobDeleteHandler(printer, target)
}

// NewRepoTagsHandler returns a repo tags handler.
func NewRepoTagsHandler(out io.Writer, format option.Format) (metadata.RepoTagsHandler, error) {
	var handler metadata.RepoTagsHandler
	switch format.Type {
	case option.FormatTypeText.Name:
		handler = text.NewRepoTagsHandler(out)
	case option.FormatTypeJSON.Name:
		handler = json.NewRepoTagsHandler(out)
	case option.FormatTypeGoTemplate.Name:
		handler = template.NewRepoTagsHandler(out, format.Template)
	default:
		return nil, errors.UnsupportedFormatTypeError(format.Type)
	}
	return handler, nil
}

// NewRepoListHandler returns a repo ls handler.
func NewRepoListHandler(out io.Writer, format option.Format, registry, namespace string) (metadata.RepoListHandler, error) {
	var handler metadata.RepoListHandler
	switch format.Type {
	case option.FormatTypeText.Name:
		handler = text.NewRepoListHandler(out, namespace)
	case option.FormatTypeJSON.Name:
		handler = json.NewRepoListHandler(out, registry)
	case option.FormatTypeGoTemplate.Name:
		handler = template.NewRepoListHandler(out, format.Template, registry)
	default:
		return nil, errors.UnsupportedFormatTypeError(format.Type)
	}
	return handler, nil
}
