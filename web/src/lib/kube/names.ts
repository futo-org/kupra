// Validation of Kubernetes object names, with apimachinery's messages.

const label = /^[a-z0-9]([-a-z0-9]*[a-z0-9])?$/;

/** dnsLabelError says why name isn't a DNS label (RFC 1123), as Previews' names must be. */
export const dnsLabelError = (name: string) => {
  if (name.length > 63) {
    return 'must be no more than 63 characters';
  }
  if (!label.test(name)) {
    return "must consist of lower case alphanumeric characters or '-', and must start and end with an alphanumeric character";
  }
};

/** dnsSubdomainError says why name isn't a DNS subdomain (RFC 1123), as most objects' names must be. */
export const dnsSubdomainError = (name: string) => {
  if (name.length > 253) {
    return 'must be no more than 253 characters';
  }
  if (name.split('.').some((part) => !label.test(part))) {
    return "must consist of lower case alphanumeric characters, '-' or '.', and must start and end with an alphanumeric character";
  }
};
