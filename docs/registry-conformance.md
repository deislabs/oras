# OCI Artifact Registry Conformance Testing

How do you know a specific registry supports [OCI Artifacts][oci-artifacts]? The following basic tests may be performed against a specific registry to validate the registry provides the extensibility in `mediaTypes`.

OCI Artifacts uses the `manifest.config.mediaType` to determine what the artifact is. This would be the equivalent of a file extension.

For a registry to be OCI Artifact capable, it would need to:

- Accept any value for `oci.image.manifest.config.mediaType`
- Accept any value for `oci.layer.mediaType`
- Accept an empty config object. See [oras config](./config.md)

For more information on defining an OCI Artifact, see [Artifact Authors Guidance](https://github.com/opencontainers/artifacts/blob/master/artifact-authors.md)

The following steps use the `oras` cli to push and pull artifacts, testing the restrictions a registry may be applying to the `mediaTypes`, and any logic around parsing the `manifest.config` object to determine platform features of a container image.

## Environment Variables

Create a few environment variables to make it easier to execute the commands with copy/paste:

```bash
REGISTRY=registry.example.io
USER=me
PASSWD=myPassword
REPO=hello-artifact
```

## Content Creation

Create a few files for testing the pushing and pulling of content.

```bash
mkdir artifact-validation
cd artifact-validation

echo "Here is an artifact" > artifact.txt
mkdir subdir-1
cd subdir-1
echo "sub directory 1 content" > artifact-1-1.txt
echo "sub directory 1 content" > artifact-1-2.txt
cd ../

mkdir subdir-2
cd subdir-2
echo "sub directory 2 content" > artifact-2-1.txt
echo "sub directory 2 content" > artifact-2-2.txt
cd ../

echo '{"version": "0.0.0.0", "name": "value"}' > artifact-config.json

mkdir output
```

## Authenticate with the Registry

Assuming the registry supports basic auth, login:

```bash
oras login -u $USER -p $PASSWD $REGISTRY
```

## Push with an Artifact Specific Config

Push a single file artifact, with an artifact specific config object.

- The `manifest.config` has something specific to the `x.sample` artifact. Since the object isn't of type `application/vnd.docker.container.image.v1+json`, all registry viewing tools should just ignore the content.
- The `manifest.config.mediaType=application/x.sample`
- The `layer.mediaType=application/txt`
- Layer content = `artifact.txt`

```bash
oras push ${REGISTRY}/$REPO:artifact-config \
  --manifest-config ./artifact-config.json:application/x.sample.v1+json \
  ./artifact.txt:application/txt

oras pull \
  --allow-all \
  --output ./output \
  ${REGISTRY}/$REPO:artifact-config

ls ./output/
rm ./output/*.*
```

## Push Empty Config, Single File

Push a single file artifact, with a null config.

- The `manifest.config` object is empty. See [oras config](./config.md) for more details.
- The `manifest.config.mediaType=application/x.sample`
- The `layer.mediaType=application/txt`
- Layer content = `artifact.txt`

```bash
oras push ${REGISTRY}/$REPO:empty-config \
  --manifest-config /dev/null:application/x.sample \
  ./artifact.txt:application/txt

oras pull \
  --allow-all \
  --output ./output \
  ${REGISTRY}/$REPO:empty-config

ls ./output/
rm ./output/*.*
```

## Push Empty Config, SubDirector

Push a single file artifact, with a null config.

- The `manifest.config` object is empty. See [oras config](./config.md) for more details.
- The `manifest.config.mediaType=application/x.sample`
- The `layer.mediaType=application/txt`
- Layer content = a sub directory of a few files, which oras will create tar.

```bash
oras push ${REGISTRY}/$REPO:multi-file-tar \
  --manifest-config /dev/null:application/x.sample \
  ./subdir-1/:application/tar

oras pull \
  --allow-all \
  --output ./output \
  ${REGISTRY}/$REPO:multi-file-tar

tree ./output/
rm -r ./output/subdir
```

## Push Empty Config, Multiple SubDirectories

Push a single file artifact, with a null config.

- The `manifest.config` object is empty. See [oras config](./config.md) for more details.
- The `manifest.config.mediaType=application/x.sample`
- The `layer.mediaType=application/txt`
- Layer contents = two sub directories of a few files, which oras will create tar as separate layers (blobs).

```bash
oras push ${REGISTRY}/$REPO:multi-dir-tar \
  --manifest-config /dev/null:application/x.sample \
  ./subdir-1/:application/tar \
  ./subdir-2/:application/tar

oras pull \
  --allow-all \
  --output ./output \
  ${REGISTRY}/$REPO:multi-dir-tar

tree ./output/
rm -r ./output/subdir
```

[oci-artifacts]:      https://github.com/opencontainers/artifacts