import { IObjectMeta } from "@kubernetes-models/apimachinery/apis/meta/v1/ObjectMeta";
import { Model, ModelData } from "@kubernetes-models/base";
/**
 * PreviewPolicy holds admin-controlled settings for previews.
 */
export interface IPreviewPolicy {
    /**
     * APIVersion defines the versioned schema of this representation of an object.
     * Servers should convert recognized schemas to the latest internal value, and
     * may reject unrecognized values.
     * More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#resources
     */
    "apiVersion": "kupra.futo.tech/v1alpha1";
    /**
     * Kind is a string value representing the REST resource this object represents.
     * Servers may infer this from the endpoint the client submits requests to.
     * Cannot be updated.
     * In CamelCase.
     * More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#types-kinds
     */
    "kind": "PreviewPolicy";
    "metadata"?: IObjectMeta;
    /**
     * PreviewPolicySpec holds the settings for preview namespaces. Fields left out are taken from the default PreviewPolicy.
     */
    "spec"?: {
        /**
         * APIServerAccess lets some pods in each preview namespace reach the Kubernetes API server, which NetworkPolicies can only allow by address. kupra creates the NetworkPolicy kupra-api-server with the addresses and ports of the kubernetes Service's endpoints, and keeps them up to date.
         */
        "apiServerAccess"?: {
            /**
             * PodSelector selects the pods that may reach the API server. An empty selector selects all pods, and leaving it out selects none.
             */
            "podSelector"?: {
                /**
                 * matchExpressions is a list of label selector requirements. The requirements are ANDed.
                 */
                "matchExpressions"?: Array<{
                    /**
                     * key is the label key that the selector applies to.
                     */
                    "key": string;
                    /**
                     * operator represents a key's relationship to a set of values.
                     * Valid operators are In, NotIn, Exists and DoesNotExist.
                     */
                    "operator": string;
                    /**
                     * values is an array of string values. If the operator is In or NotIn,
                     * the values array must be non-empty. If the operator is Exists or DoesNotExist,
                     * the values array must be empty. This array is replaced during a strategic
                     * merge patch.
                     */
                    "values"?: Array<string>;
                }>;
                /**
                 * matchLabels is a map of {key,value} pairs. A single {key,value} in the matchLabels
                 * map is equivalent to an element of matchExpressions, whose key field is "key", the
                 * operator is "In", and the values array contains only "value". The requirements are ANDed.
                 */
                "matchLabels"?: {
                    [key: string]: string;
                };
            };
        };
        /**
         * Apply is how templates are applied.
         */
        "apply"?: {
            /**
             * ClusterRoleRef names the ClusterRole templates are applied with.
             */
            "clusterRoleRef": {
                /**
                 * Name of the referent.
                 */
                "name": string;
            };
        };
        /**
         * LimitRange is created in each preview namespace.
         */
        "limitRange"?: {
            /**
             * Limits is the list of LimitRangeItem objects that are enforced.
             */
            "limits": Array<{
                /**
                 * Default resource requirement limit value by resource name if resource limit is omitted.
                 */
                "default"?: {
                    [key: string]: number | string;
                };
                /**
                 * DefaultRequest is the default resource requirement request value by resource name if resource request is omitted.
                 */
                "defaultRequest"?: {
                    [key: string]: number | string;
                };
                /**
                 * Max usage constraints on this kind by resource name.
                 */
                "max"?: {
                    [key: string]: number | string;
                };
                /**
                 * MaxLimitRequestRatio if specified, the named resource must have a request and limit that are both non-zero where limit divided by request is less than or equal to the enumerated value; this represents the max burst for the named resource.
                 */
                "maxLimitRequestRatio"?: {
                    [key: string]: number | string;
                };
                /**
                 * Min usage constraints on this kind by resource name.
                 */
                "min"?: {
                    [key: string]: number | string;
                };
                /**
                 * Type of resource that this limit applies to.
                 */
                "type": string;
            }>;
        };
        /**
         * NamespaceMetadata adds labels and annotations to each preview namespace, merged over the default PreviewPolicy's.
         */
        "namespaceMetadata"?: {
            /**
             * Annotations are added to each preview namespace.
             */
            "annotations"?: {
                [key: string]: string;
            };
            /**
             * Labels are added to each preview namespace.
             */
            "labels"?: {
                [key: string]: string;
            };
        };
        /**
         * NetworkPolicies are created in each preview namespace. An empty list creates none.
         */
        "networkPolicies"?: Array<{
            /**
             * Metadata is the NetworkPolicy's metadata.
             */
            "metadata": {
                /**
                 * Annotations are added to the object.
                 */
                "annotations"?: {
                    [key: string]: string;
                };
                /**
                 * Labels are added to the object.
                 */
                "labels"?: {
                    [key: string]: string;
                };
                /**
                 * Name is the object's name.
                 */
                "name": string;
            };
            /**
             * Spec is the NetworkPolicy's spec.
             */
            "spec": {
                /**
                 * egress is a list of egress rules to be applied to the selected pods. Outgoing traffic
                 * is allowed if there are no NetworkPolicies selecting the pod (and cluster policy
                 * otherwise allows the traffic), OR if the traffic matches at least one egress rule
                 * across all of the NetworkPolicy objects whose podSelector matches the pod. If
                 * this field is empty then this NetworkPolicy limits all outgoing traffic (and serves
                 * solely to ensure that the pods it selects are isolated by default).
                 * This field is beta-level in 1.8
                 */
                "egress"?: Array<{
                    /**
                     * ports is a list of destination ports for outgoing traffic.
                     * Each item in this list is combined using a logical OR. If this field is
                     * empty or missing, this rule matches all ports (traffic not restricted by port).
                     * If this field is present and contains at least one item, then this rule allows
                     * traffic only if the traffic matches at least one port in the list.
                     */
                    "ports"?: Array<{
                        /**
                         * endPort indicates that the range of ports from port to endPort if set, inclusive,
                         * should be allowed by the policy. This field cannot be defined if the port field
                         * is not defined or if the port field is defined as a named (string) port.
                         * The endPort must be equal or greater than port.
                         */
                        "endPort"?: number;
                        /**
                         * port represents the port on the given protocol. This can either be a numerical or named
                         * port on a pod. If this field is not provided, this matches all port names and
                         * numbers.
                         * If present, only traffic on the specified protocol AND port will be matched.
                         */
                        "port"?: number | string;
                        /**
                         * protocol represents the protocol (TCP, UDP, or SCTP) which traffic must match.
                         * If not specified, this field defaults to TCP.
                         */
                        "protocol"?: string;
                    }>;
                    /**
                     * to is a list of destinations for outgoing traffic of pods selected for this rule.
                     * Items in this list are combined using a logical OR operation. If this field is
                     * empty or missing, this rule matches all destinations (traffic not restricted by
                     * destination). If this field is present and contains at least one item, this rule
                     * allows traffic only if the traffic matches at least one item in the to list.
                     */
                    "to"?: Array<{
                        /**
                         * ipBlock defines policy on a particular IPBlock. If this field is set then
                         * neither of the other fields can be.
                         */
                        "ipBlock"?: {
                            /**
                             * cidr is a string representing the IPBlock
                             * Valid examples are "192.168.1.0/24" or "2001:db8::/64"
                             */
                            "cidr": string;
                            /**
                             * except is a slice of CIDRs that should not be included within an IPBlock
                             * Valid examples are "192.168.1.0/24" or "2001:db8::/64"
                             * Except values will be rejected if they are outside the cidr range
                             */
                            "except"?: Array<string>;
                        };
                        /**
                         * namespaceSelector selects namespaces using cluster-scoped labels. This field follows
                         * standard label selector semantics; if present but empty, it selects all namespaces.
                         *
                         * If podSelector is also set, then the NetworkPolicyPeer as a whole selects
                         * the pods matching podSelector in the namespaces selected by namespaceSelector.
                         * Otherwise it selects all pods in the namespaces selected by namespaceSelector.
                         */
                        "namespaceSelector"?: {
                            /**
                             * matchExpressions is a list of label selector requirements. The requirements are ANDed.
                             */
                            "matchExpressions"?: Array<{
                                /**
                                 * key is the label key that the selector applies to.
                                 */
                                "key": string;
                                /**
                                 * operator represents a key's relationship to a set of values.
                                 * Valid operators are In, NotIn, Exists and DoesNotExist.
                                 */
                                "operator": string;
                                /**
                                 * values is an array of string values. If the operator is In or NotIn,
                                 * the values array must be non-empty. If the operator is Exists or DoesNotExist,
                                 * the values array must be empty. This array is replaced during a strategic
                                 * merge patch.
                                 */
                                "values"?: Array<string>;
                            }>;
                            /**
                             * matchLabels is a map of {key,value} pairs. A single {key,value} in the matchLabels
                             * map is equivalent to an element of matchExpressions, whose key field is "key", the
                             * operator is "In", and the values array contains only "value". The requirements are ANDed.
                             */
                            "matchLabels"?: {
                                [key: string]: string;
                            };
                        };
                        /**
                         * podSelector is a label selector which selects pods. This field follows standard label
                         * selector semantics; if present but empty, it selects all pods.
                         *
                         * If namespaceSelector is also set, then the NetworkPolicyPeer as a whole selects
                         * the pods matching podSelector in the Namespaces selected by NamespaceSelector.
                         * Otherwise it selects the pods matching podSelector in the policy's own namespace.
                         */
                        "podSelector"?: {
                            /**
                             * matchExpressions is a list of label selector requirements. The requirements are ANDed.
                             */
                            "matchExpressions"?: Array<{
                                /**
                                 * key is the label key that the selector applies to.
                                 */
                                "key": string;
                                /**
                                 * operator represents a key's relationship to a set of values.
                                 * Valid operators are In, NotIn, Exists and DoesNotExist.
                                 */
                                "operator": string;
                                /**
                                 * values is an array of string values. If the operator is In or NotIn,
                                 * the values array must be non-empty. If the operator is Exists or DoesNotExist,
                                 * the values array must be empty. This array is replaced during a strategic
                                 * merge patch.
                                 */
                                "values"?: Array<string>;
                            }>;
                            /**
                             * matchLabels is a map of {key,value} pairs. A single {key,value} in the matchLabels
                             * map is equivalent to an element of matchExpressions, whose key field is "key", the
                             * operator is "In", and the values array contains only "value". The requirements are ANDed.
                             */
                            "matchLabels"?: {
                                [key: string]: string;
                            };
                        };
                    }>;
                }>;
                /**
                 * ingress is a list of ingress rules to be applied to the selected pods.
                 * Traffic is allowed to a pod if there are no NetworkPolicies selecting the pod
                 * (and cluster policy otherwise allows the traffic), OR if the traffic source is
                 * the pod's local node, OR if the traffic matches at least one ingress rule
                 * across all of the NetworkPolicy objects whose podSelector matches the pod. If
                 * this field is empty then this NetworkPolicy does not allow any traffic (and serves
                 * solely to ensure that the pods it selects are isolated by default)
                 */
                "ingress"?: Array<{
                    /**
                     * from is a list of sources which should be able to access the pods selected for this rule.
                     * Items in this list are combined using a logical OR operation. If this field is
                     * empty or missing, this rule matches all sources (traffic not restricted by
                     * source). If this field is present and contains at least one item, this rule
                     * allows traffic only if the traffic matches at least one item in the from list.
                     */
                    "from"?: Array<{
                        /**
                         * ipBlock defines policy on a particular IPBlock. If this field is set then
                         * neither of the other fields can be.
                         */
                        "ipBlock"?: {
                            /**
                             * cidr is a string representing the IPBlock
                             * Valid examples are "192.168.1.0/24" or "2001:db8::/64"
                             */
                            "cidr": string;
                            /**
                             * except is a slice of CIDRs that should not be included within an IPBlock
                             * Valid examples are "192.168.1.0/24" or "2001:db8::/64"
                             * Except values will be rejected if they are outside the cidr range
                             */
                            "except"?: Array<string>;
                        };
                        /**
                         * namespaceSelector selects namespaces using cluster-scoped labels. This field follows
                         * standard label selector semantics; if present but empty, it selects all namespaces.
                         *
                         * If podSelector is also set, then the NetworkPolicyPeer as a whole selects
                         * the pods matching podSelector in the namespaces selected by namespaceSelector.
                         * Otherwise it selects all pods in the namespaces selected by namespaceSelector.
                         */
                        "namespaceSelector"?: {
                            /**
                             * matchExpressions is a list of label selector requirements. The requirements are ANDed.
                             */
                            "matchExpressions"?: Array<{
                                /**
                                 * key is the label key that the selector applies to.
                                 */
                                "key": string;
                                /**
                                 * operator represents a key's relationship to a set of values.
                                 * Valid operators are In, NotIn, Exists and DoesNotExist.
                                 */
                                "operator": string;
                                /**
                                 * values is an array of string values. If the operator is In or NotIn,
                                 * the values array must be non-empty. If the operator is Exists or DoesNotExist,
                                 * the values array must be empty. This array is replaced during a strategic
                                 * merge patch.
                                 */
                                "values"?: Array<string>;
                            }>;
                            /**
                             * matchLabels is a map of {key,value} pairs. A single {key,value} in the matchLabels
                             * map is equivalent to an element of matchExpressions, whose key field is "key", the
                             * operator is "In", and the values array contains only "value". The requirements are ANDed.
                             */
                            "matchLabels"?: {
                                [key: string]: string;
                            };
                        };
                        /**
                         * podSelector is a label selector which selects pods. This field follows standard label
                         * selector semantics; if present but empty, it selects all pods.
                         *
                         * If namespaceSelector is also set, then the NetworkPolicyPeer as a whole selects
                         * the pods matching podSelector in the Namespaces selected by NamespaceSelector.
                         * Otherwise it selects the pods matching podSelector in the policy's own namespace.
                         */
                        "podSelector"?: {
                            /**
                             * matchExpressions is a list of label selector requirements. The requirements are ANDed.
                             */
                            "matchExpressions"?: Array<{
                                /**
                                 * key is the label key that the selector applies to.
                                 */
                                "key": string;
                                /**
                                 * operator represents a key's relationship to a set of values.
                                 * Valid operators are In, NotIn, Exists and DoesNotExist.
                                 */
                                "operator": string;
                                /**
                                 * values is an array of string values. If the operator is In or NotIn,
                                 * the values array must be non-empty. If the operator is Exists or DoesNotExist,
                                 * the values array must be empty. This array is replaced during a strategic
                                 * merge patch.
                                 */
                                "values"?: Array<string>;
                            }>;
                            /**
                             * matchLabels is a map of {key,value} pairs. A single {key,value} in the matchLabels
                             * map is equivalent to an element of matchExpressions, whose key field is "key", the
                             * operator is "In", and the values array contains only "value". The requirements are ANDed.
                             */
                            "matchLabels"?: {
                                [key: string]: string;
                            };
                        };
                    }>;
                    /**
                     * ports is a list of ports which should be made accessible on the pods selected for
                     * this rule. Each item in this list is combined using a logical OR. If this field is
                     * empty or missing, this rule matches all ports (traffic not restricted by port).
                     * If this field is present and contains at least one item, then this rule allows
                     * traffic only if the traffic matches at least one port in the list.
                     */
                    "ports"?: Array<{
                        /**
                         * endPort indicates that the range of ports from port to endPort if set, inclusive,
                         * should be allowed by the policy. This field cannot be defined if the port field
                         * is not defined or if the port field is defined as a named (string) port.
                         * The endPort must be equal or greater than port.
                         */
                        "endPort"?: number;
                        /**
                         * port represents the port on the given protocol. This can either be a numerical or named
                         * port on a pod. If this field is not provided, this matches all port names and
                         * numbers.
                         * If present, only traffic on the specified protocol AND port will be matched.
                         */
                        "port"?: number | string;
                        /**
                         * protocol represents the protocol (TCP, UDP, or SCTP) which traffic must match.
                         * If not specified, this field defaults to TCP.
                         */
                        "protocol"?: string;
                    }>;
                }>;
                /**
                 * podSelector selects the pods to which this NetworkPolicy object applies.
                 * The array of rules is applied to any pods selected by this field. An empty
                 * selector matches all pods in the policy's namespace.
                 * Multiple network policies can select the same set of pods. In this case,
                 * the ingress rules for each are combined additively.
                 * This field is optional. If it is not specified, it defaults to an empty selector.
                 */
                "podSelector"?: {
                    /**
                     * matchExpressions is a list of label selector requirements. The requirements are ANDed.
                     */
                    "matchExpressions"?: Array<{
                        /**
                         * key is the label key that the selector applies to.
                         */
                        "key": string;
                        /**
                         * operator represents a key's relationship to a set of values.
                         * Valid operators are In, NotIn, Exists and DoesNotExist.
                         */
                        "operator": string;
                        /**
                         * values is an array of string values. If the operator is In or NotIn,
                         * the values array must be non-empty. If the operator is Exists or DoesNotExist,
                         * the values array must be empty. This array is replaced during a strategic
                         * merge patch.
                         */
                        "values"?: Array<string>;
                    }>;
                    /**
                     * matchLabels is a map of {key,value} pairs. A single {key,value} in the matchLabels
                     * map is equivalent to an element of matchExpressions, whose key field is "key", the
                     * operator is "In", and the values array contains only "value". The requirements are ANDed.
                     */
                    "matchLabels"?: {
                        [key: string]: string;
                    };
                };
                /**
                 * policyTypes is a list of rule types that the NetworkPolicy relates to.
                 * Valid options are ["Ingress"], ["Egress"], or ["Ingress", "Egress"].
                 * If this field is not specified, it will default based on the existence of ingress or egress rules;
                 * policies that contain an egress section are assumed to affect egress, and all policies
                 * (whether or not they contain an ingress section) are assumed to affect ingress.
                 * If you want to write an egress-only policy, you must explicitly specify policyTypes [ "Egress" ].
                 * Likewise, if you want to write a policy that specifies that no egress is allowed,
                 * you must specify a policyTypes value that include "Egress" (since such a policy would not include
                 * an egress section and would otherwise default to just [ "Ingress" ]).
                 * This field is beta-level in 1.8
                 */
                "policyTypes"?: Array<string>;
            };
        }>;
        /**
         * ResourceQuota is created in each preview namespace.
         */
        "resourceQuota"?: {
            /**
             * hard is the set of desired hard limits for each named resource.
             * More info: https://kubernetes.io/docs/concepts/policy/resource-quotas/
             */
            "hard"?: {
                [key: string]: number | string;
            };
            /**
             * scopeSelector is also a collection of filters like scopes that must match each object tracked by a quota
             * but expressed using ScopeSelectorOperator in combination with possible values.
             * For a resource to match, both scopes AND scopeSelector (if specified in spec), must be matched.
             */
            "scopeSelector"?: {
                /**
                 * A list of scope selector requirements by scope of the resources.
                 */
                "matchExpressions"?: Array<{
                    /**
                     * Represents a scope's relationship to a set of values.
                     * Valid operators are In, NotIn, Exists, DoesNotExist.
                     */
                    "operator": string;
                    /**
                     * The name of the scope that the selector applies to.
                     */
                    "scopeName": string;
                    /**
                     * An array of string values. If the operator is In or NotIn,
                     * the values array must be non-empty. If the operator is Exists or DoesNotExist,
                     * the values array must be empty.
                     * This array is replaced during a strategic merge patch.
                     */
                    "values"?: Array<string>;
                }>;
            };
            /**
             * A collection of filters that must match each object tracked by a quota.
             * If not specified, the quota matches all objects.
             */
            "scopes"?: Array<string>;
        };
    };
    /**
     * Status is the observed state of the PreviewPolicy.
     */
    "status"?: {
        /**
         * Conditions holds the conditions for the PreviewPolicy.
         */
        "conditions"?: Array<{
            /**
             * lastTransitionTime is the last time the condition transitioned from one status to another.
             * This should be when the underlying condition changed.  If that is not known, then using the time when the API field changed is acceptable.
             */
            "lastTransitionTime": string;
            /**
             * message is a human readable message indicating details about the transition.
             * This may be an empty string.
             */
            "message": string;
            /**
             * observedGeneration represents the .metadata.generation that the condition was set based upon.
             * For instance, if .metadata.generation is currently 12, but the .status.conditions[x].observedGeneration is 9, the condition is out of date
             * with respect to the current state of the instance.
             */
            "observedGeneration"?: number;
            /**
             * reason contains a programmatic identifier indicating the reason for the condition's last transition.
             * Producers of specific condition types may define expected values and meanings for this field,
             * and whether the values are considered a guaranteed API.
             * The value should be a CamelCase string.
             * This field may not be empty.
             */
            "reason": string;
            /**
             * status of the condition, one of True, False, Unknown.
             */
            "status": "True" | "False" | "Unknown";
            /**
             * type of condition in CamelCase or in foo.example.com/CamelCase.
             */
            "type": string;
        }>;
        /**
         * ObservedGeneration is the last observed generation.
         */
        "observedGeneration"?: number;
    };
}
/**
 * PreviewPolicy holds admin-controlled settings for previews.
 */
export declare class PreviewPolicy extends Model<IPreviewPolicy> implements IPreviewPolicy {
    "apiVersion": IPreviewPolicy["apiVersion"];
    "kind": IPreviewPolicy["kind"];
    "metadata"?: IPreviewPolicy["metadata"];
    "spec"?: IPreviewPolicy["spec"];
    "status"?: IPreviewPolicy["status"];
    static apiVersion: IPreviewPolicy["apiVersion"];
    static kind: IPreviewPolicy["kind"];
    static is: import("@kubernetes-models/base").TypeMetaGuard<IPreviewPolicy>;
    constructor(data?: ModelData<IPreviewPolicy>);
}
