FROM golang:1.19 as builder

WORKDIR /src/
COPY *.go go.mod go.sum ./
COPY kafkautils ./kafkautils
COPY message ./message
RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -o ia2 ./

# Copy from the builder to keep the final image reproducible and small.  If we
# don't do this, we end up with non-deterministic build artifacts.
FROM scratch
COPY --from=builder /src/ia2 /
EXPOSE 8080
# Switch to the UID that's typically reserved for the user "nobody".
USER 65534

CMD ["/ia2"]
