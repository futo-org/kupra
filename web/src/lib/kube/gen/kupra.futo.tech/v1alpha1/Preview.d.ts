import { IObjectMeta } from "@kubernetes-models/apimachinery/apis/meta/v1/ObjectMeta";
import { Model, ModelData } from "@kubernetes-models/base";
/**
 * Preview is a running instance of a PreviewApp.
 */
export interface IPreview {
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
    "kind": "Preview";
    "metadata"?: IObjectMeta;
    /**
     * Spec is the desired state of the Preview.
     */
    "spec": {
        /**
         * AppRef names the PreviewApp in the Preview's namespace.
         */
        "appRef": {
            /**
             * Name of the referent.
             */
            "name": string;
        };
        /**
         * Owners can manage the preview, along with whoever created it. As in RoleBindings, apiGroup defaults to rbac.authorization.k8s.io for users and groups.
         */
        "owners"?: Array<{
            /**
             * apiGroup holds the API group of the referenced subject.
             * Defaults to "" for ServiceAccount subjects.
             * Defaults to "rbac.authorization.k8s.io" for User and Group subjects.
             */
            "apiGroup"?: string;
            /**
             * kind of object being referenced. Values defined by this API group are "User", "Group", and "ServiceAccount".
             * If the Authorizer does not recognized the kind value, the Authorizer should report an error.
             */
            "kind": string;
            /**
             * name of the object being referenced.
             */
            "name": string;
            /**
             * namespace of the referenced object.  If the object kind is non-namespace, such as "User" or "Group", and this value is not empty
             * the Authorizer should report an error.
             */
            "namespace"?: string;
        }>;
        /**
         * Parameters are values for the PreviewApp's parameters. Like environment variables, they're strings, whatever the parameter's type.
         */
        "parameters"?: {
            [key: string]: string;
        };
        /**
         * RestoreFrom is the snapshot the preview's state starts from, when it starts or is reset. Without it, state starts empty.
         */
        "restoreFrom"?: {
            /**
             * SnapshotRef names a PreviewSnapshot in the preview's namespace.
             */
            "snapshotRef"?: {
                /**
                 * Name of the referent.
                 */
                "name": string;
            };
            /**
             * SnapshotSelector selects the newest ready PreviewSnapshot in the preview's namespace.
             */
            "snapshotSelector"?: {
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
    };
    /**
     * Status is the observed state of the Preview.
     */
    "status"?: {
        /**
         * Conditions holds the conditions for the Preview.
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
         * Inventory lists the resources kupra applied for the preview.
         */
        "inventory"?: Array<{
            /**
             * APIGroup is the group for the resource being referenced.
             * If APIGroup is not specified, the specified Kind must be in the core API group.
             * For any other third-party types, APIGroup is required.
             */
            "apiGroup"?: string;
            /**
             * Kind is the type of resource being referenced
             */
            "kind": string;
            /**
             * Name is the name of resource being referenced
             */
            "name": string;
            /**
             * Retained is set for state that's no longer rendered, until the preview is reset.
             */
            "retained"?: boolean;
            /**
             * State is set for state, which is kept when it's no longer rendered.
             */
            "state"?: boolean;
        }>;
        /**
         * LastHandledResetAt is the last handled value of the kupra.futo.tech/reset-requested-at annotation.
         */
        "lastHandledResetAt"?: string;
        /**
         * Links are the PreviewApp's links for this preview.
         */
        "links"?: Array<{
            /**
             * Name is the link's label.
             */
            "name": string;
            /**
             * URL is the link's URL.
             */
            "url": string;
        }>;
        /**
         * Namespace is the namespace the preview's resources run in.
         */
        "namespace"?: string;
        /**
         * ObservedGeneration is the last observed generation.
         */
        "observedGeneration"?: number;
        /**
         * Phase summarises the state for display. Tools should use conditions instead.
         */
        "phase"?: "Pending" | "Provisioning" | "Starting" | "Running" | "Stopping" | "Stopped" | "Terminating" | "Failed";
        /**
         * RestoredFrom is the PreviewSnapshot the preview's state started from.
         */
        "restoredFrom"?: {
            /**
             * Name of the referent.
             */
            "name": string;
        };
    };
}
/**
 * Preview is a running instance of a PreviewApp.
 */
export declare class Preview extends Model<IPreview> implements IPreview {
    "apiVersion": IPreview["apiVersion"];
    "kind": IPreview["kind"];
    "metadata"?: IPreview["metadata"];
    "spec": IPreview["spec"];
    "status"?: IPreview["status"];
    static apiVersion: IPreview["apiVersion"];
    static kind: IPreview["kind"];
    static is: import("@kubernetes-models/base").TypeMetaGuard<IPreview>;
    constructor(data?: ModelData<IPreview>);
}
