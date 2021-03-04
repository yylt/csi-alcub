FROM gcr.io/distroless/static:latest
ARG binary=./bin/hyper

COPY ${binary} hyper
ENTRYPOINT ["/hyper"]
