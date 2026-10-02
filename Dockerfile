FROM gcr.io/distroless/static:nonroot
WORKDIR /
COPY mcp-kubernetes-ro /mcp-kubernetes-ro
# The container cannot write files to the user's disk, so Secret values are
# delivered encrypted instead of through save_secret_to_file.
ENV MCP_KUBERNETES_RO_ENCRYPTED_SECRET_ACCESS=true
ENTRYPOINT ["/mcp-kubernetes-ro"]
