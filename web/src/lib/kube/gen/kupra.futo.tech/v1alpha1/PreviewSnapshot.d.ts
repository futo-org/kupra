import { IObjectMeta } from "@kubernetes-models/apimachinery/apis/meta/v1/ObjectMeta";
import { Model, ModelData } from "@kubernetes-models/base";
/**
 * PreviewSnapshot is a captured copy of a preview's state.
 */
export interface IPreviewSnapshot {
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
    "kind": "PreviewSnapshot";
    "metadata"?: IObjectMeta;
    /**
     * Spec is the desired state of the PreviewSnapshot.
     */
    "spec": {
        /**
         * Mode is how the snapshot is kept consistent: Offline stops the preview's app while capturing, and Online doesn't.
         */
        "mode"?: "Offline" | "Online";
        /**
         * Source is the preview to capture.
         */
        "source": {
            /**
             * PreviewRef names the Preview to capture, in the snapshot's namespace.
             */
            "previewRef": {
                /**
                 * Name of the referent.
                 */
                "name": string;
            };
        };
    };
    /**
     * Status is the observed state of the PreviewSnapshot.
     */
    "status"?: {
        /**
         * AppRef is the PreviewApp the source preview ran.
         */
        "appRef"?: {
            /**
             * Name of the referent.
             */
            "name": string;
        };
        /**
         * Captures are the captured state resources.
         */
        "captures"?: Array<{
            /**
             * Data are the objects in the snapshot's namespace that hold the capture, such as VolumeSnapshots.
             */
            "data": Array<{
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
            }>;
            /**
             * Group is the resource's API group.
             */
            "group": string;
            /**
             * Kind is the resource's kind.
             */
            "kind": string;
            /**
             * Name is the resource's name.
             */
            "name": string;
        }>;
        /**
         * CompletionTime is when the capture succeeded or failed.
         */
        "completionTime"?: string;
        /**
         * Conditions holds the conditions for the PreviewSnapshot.
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
        /**
         * Parameters are the source preview's parameters when captured.
         */
        "parameters"?: {
            [key: string]: string;
        };
        /**
         * Phase summarises the state for display. Tools should use conditions instead.
         */
        "phase"?: "InProgress" | "Succeeded" | "Failed" | "Deleting";
        /**
         * ReadyToUse indicates if previews can start from the snapshot.
         */
        "readyToUse"?: boolean;
        /**
         * StartTime is when the capture started.
         */
        "startTime"?: string;
    };
}
/**
 * PreviewSnapshot is a captured copy of a preview's state.
 */
export declare class PreviewSnapshot extends Model<IPreviewSnapshot> implements IPreviewSnapshot {
    "apiVersion": IPreviewSnapshot["apiVersion"];
    "kind": IPreviewSnapshot["kind"];
    "metadata"?: IPreviewSnapshot["metadata"];
    "spec": IPreviewSnapshot["spec"];
    "status"?: IPreviewSnapshot["status"];
    static apiVersion: IPreviewSnapshot["apiVersion"];
    static kind: IPreviewSnapshot["kind"];
    static is: import("@kubernetes-models/base").TypeMetaGuard<IPreviewSnapshot>;
    constructor(data?: ModelData<IPreviewSnapshot>);
}
