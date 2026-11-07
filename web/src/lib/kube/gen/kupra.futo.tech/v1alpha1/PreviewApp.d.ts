import { IObjectMeta } from "@kubernetes-models/apimachinery/apis/meta/v1/ObjectMeta";
import { Model, ModelData } from "@kubernetes-models/base";
/**
 * PreviewApp describes how to run an app as a preview.
 */
export interface IPreviewApp {
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
    "kind": "PreviewApp";
    "metadata"?: IObjectMeta;
    /**
     * Spec is the desired state of the PreviewApp.
     */
    "spec": {
        /**
         * Links are shown for each preview, evaluated into its status.
         */
        "links"?: Array<{
            /**
             * Name is the link's label.
             */
            "name": string;
            /**
             * URLExpr is a CEL expression that evaluates to the link's URL.
             */
            "urlExpr": string;
        }>;
        /**
         * Parameters are a preview's inputs, in the order the dashboard shows them.
         */
        "parameters"?: Array<{
            /**
             * Default is used when a Preview gives no value. Optional parameters without a default are the empty value of their type.
             */
            "default"?: string;
            /**
             * Description is the parameter's help text in the dashboard.
             */
            "description"?: string;
            /**
             * Enum limits the parameter to these values.
             */
            "enum"?: Array<string>;
            /**
             * Name is the parameter's name in expressions and in a Preview's parameters.
             */
            "name": string;
            /**
             * Required means a Preview must give the parameter a value, unless it has a default.
             */
            "required"?: boolean;
            /**
             * Title is the parameter's label in the dashboard.
             */
            "title": string;
            /**
             * Type is the parameter's value type.
             */
            "type"?: "string" | "integer" | "boolean";
            /**
             * UIHint asks the dashboard for a special input, such as a picker for an image's tags.
             */
            "uiHint"?: {
                /**
                 * ImageTag configures the imageTag hint.
                 */
                "imageTag"?: {
                    /**
                     * Repository is the image repository whose tags are offered.
                     */
                    "repository": string;
                };
                /**
                 * Type is the kind of input the dashboard shows.
                 */
                "type": "imageTag";
            };
        }>;
        /**
         * Template is what each preview runs.
         */
        "template": {
            /**
             * Resources are the manifests rendered for each preview. Any field can be given as a CEL expression by adding Expr to its name.
             */
            "resources": Array<{}>;
        };
        /**
         * Timeout is how long a preview may take to become ready before it's marked as stalled. Defaults to 30m.
         */
        "timeout"?: string;
        /**
         * Variables are values computed once per preview, each from the parameters, the preview and the variables before it.
         */
        "variables"?: Array<{
            /**
             * Expression is a CEL expression.
             */
            "expression": string;
            /**
             * Name is the variable's name in expressions.
             */
            "name": string;
        }>;
    };
    /**
     * Status is the observed state of the PreviewApp.
     */
    "status"?: {
        /**
         * Conditions holds the conditions for the PreviewApp.
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
 * PreviewApp describes how to run an app as a preview.
 */
export declare class PreviewApp extends Model<IPreviewApp> implements IPreviewApp {
    "apiVersion": IPreviewApp["apiVersion"];
    "kind": IPreviewApp["kind"];
    "metadata"?: IPreviewApp["metadata"];
    "spec": IPreviewApp["spec"];
    "status"?: IPreviewApp["status"];
    static apiVersion: IPreviewApp["apiVersion"];
    static kind: IPreviewApp["kind"];
    static is: import("@kubernetes-models/base").TypeMetaGuard<IPreviewApp>;
    constructor(data?: ModelData<IPreviewApp>);
}
