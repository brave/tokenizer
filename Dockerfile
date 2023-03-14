FROM public.ecr.aws/docker/library/golang:1.20 as builder

WORKDIR /src/
COPY *.go go.mod go.sum ./
RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -o tkzr ./

# Copy from the builder to keep the final image reproducible and small.  If we
# don't do this, we end up with non-deterministic build artifacts.
FROM scratch

COPY --from=builder /src/tkzr /bin/
EXPOSE 8080

CMD ["/bin/tkzr"]
