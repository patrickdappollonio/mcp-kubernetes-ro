FROM gcr.io/distroless/static:nonroot
WORKDIR /
COPY mcp-kubernetes-ro /mcp-kubernetes-ro
ENTRYPOINT ["/mcp-kubernetes-ro"]
