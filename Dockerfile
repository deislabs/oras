# Copyright The ORAS Authors.
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
# http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

FROM --platform=$BUILDPLATFORM docker.io/library/golang:1.27.3-alpine as builder
ARG TARGETPLATFORM
RUN apk add git make
ENV ORASPKG /oras
ADD . ${ORASPKG}
WORKDIR ${ORASPKG}
RUN make "build-$(echo $TARGETPLATFORM | tr / -)"
RUN mv ${ORASPKG}/bin/${TARGETPLATFORM}/oras /go/bin/oras

FROM docker.io/library/alpine:3.24.3
ARG OCI_VERSION=dev
ARG OCI_REVISION=unknown
LABEL org.opencontainers.image.title="ORAS CLI" \
      org.opencontainers.image.description="OCI registry client for managing artifacts" \
      org.opencontainers.image.source="https://github.com/oras-project/oras" \
      org.opencontainers.image.url="https://oras.land/" \
      org.opencontainers.image.documentation="https://oras.land/docs/" \
      org.opencontainers.image.vendor="ORAS Project" \
      org.opencontainers.image.licenses="Apache-2.0" \
      org.opencontainers.image.version="${OCI_VERSION}" \
      org.opencontainers.image.revision="${OCI_REVISION}"
RUN apk --update add ca-certificates
COPY --from=builder /go/bin/oras /bin/oras
RUN mkdir /workspace
WORKDIR /workspace
ENTRYPOINT  ["/bin/oras"]
