FROM library/golang:1.14-alpine AS build
WORKDIR /go/src/github.com/vsliouniaev/k8s-crdguard
COPY . .
ENV CGO_ENABLED=0
RUN go build -mod vendor -o main
RUN mv main /main

FROM scratch
EXPOSE 8443
COPY --from=build /main /guard
ENTRYPOINT ["/guard"]