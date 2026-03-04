import * as cdk from 'aws-cdk-lib';
import * as lambda from 'aws-cdk-lib/aws-lambda';
import * as events from 'aws-cdk-lib/aws-events';
import * as targets from 'aws-cdk-lib/aws-events-targets';
import * as apigateway from 'aws-cdk-lib/aws-apigateway';
import * as iam from 'aws-cdk-lib/aws-iam';
import * as path from 'path';
import {Bucket} from "aws-cdk-lib/aws-s3";
import {BucketDeployment, Source} from "aws-cdk-lib/aws-s3-deployment";
import {Duration} from "aws-cdk-lib";

export class BirthdayWisherStack extends cdk.Stack {
    constructor(scope: cdk.App, id: string, props?: cdk.StackProps) {
        super(scope, id, props);

        // Lambda function for birthday handling
        const birthdayHandler = new lambda.Function(this, 'BirthdayHandler', {
            runtime: lambda.Runtime.PROVIDED_AL2,
            handler: 'not.used',
            code: lambda.Code.fromAsset(path.join(__dirname, '../../lambda'), {
                bundling: {
                    command: [
                        'bash', '-c',
                        'GOOS=linux GOARCH=amd64 go build -o /asset-output/bootstrap .',
                    ],
                    image: lambda.Runtime.PROVIDED_AL2.bundlingImage,
                    user: 'root',
                    environment: {
                        GOPATH: '/go',
                        GO111MODULE: 'on'
                    },
                    workingDirectory: '/asset-input/birthday',
                },
            }),
            environment: {
                SSM_OPENAI_API_KEY: '/bday-wisher/openai-api-key',
                SSM_TWILIO_ACCOUNT_SID: '/bday-wisher/twilio-account-sid',
                SSM_TWILIO_AUTH_TOKEN: '/bday-wisher/twilio-auth-token',
                SSM_TWILIO_PHONE_NUMBER: '/bday-wisher/twilio-phone-number',
            },
            timeout: Duration.seconds(30),
            memorySize: 128
        });

        // Create an S3 bucket to store csv file
        const dataBucket = new Bucket(this, 'BirthdayWisher', {
            bucketName: 'birthday-wisher',
        });
        new BucketDeployment(this, "DeployLocalFile", {
            destinationBucket: dataBucket,
            sources: [
                Source.asset("../data")
            ],
        });

        dataBucket.grantRead(birthdayHandler.role!);

        // EventBridge rules to trigger lambda daily
        const winterRule = new events.Rule(this, 'WinterDailyBirthdayCheck', {
            schedule: events.Schedule.cron({ minute: '1', hour: '2', month: '11,12,1,2,3' })
        });
        winterRule.addTarget(new targets.LambdaFunction(birthdayHandler));
        const summerRule = new events.Rule(this, 'SummerDailyBirthdayCheck', {
            schedule: events.Schedule.cron({ minute: '1', hour: '1', month: '4,5,6,7,8,9,10' })
        });
        summerRule.addTarget(new targets.LambdaFunction(birthdayHandler));

        // Create an API Gateway to receive Twilio messages
        const api = new apigateway.RestApi(this, 'TwilioApi', {
            restApiName: 'Twilio Message API',
            description: 'API for receiving messages from Twilio.',
        });

        // Create the Lambda function for handling replies
        const replyHandler = new lambda.Function(this, 'ReplyHandler', {
            runtime: lambda.Runtime.PROVIDED_AL2,
            handler: 'bootstrap',
            code: lambda.Code.fromAsset(path.join(__dirname, '../../lambda'), {
                bundling: {
                    command: [
                        'bash', '-c',
                        'GOOS=linux GOARCH=amd64 go build -o /asset-output/bootstrap . && chmod +x /asset-output/bootstrap .',
                    ],
                    image: lambda.Runtime.PROVIDED_AL2.bundlingImage,
                    user: 'root',
                    environment: {
                        GOPATH: '/go',
                        GO111MODULE: 'on'
                    },
                    workingDirectory: '/asset-input/replier',
                },
            }),
            environment: {
                SSM_OPENAI_API_KEY: '/bday-wisher/openai-api-key',
                SSM_TWILIO_ACCOUNT_SID: '/bday-wisher/twilio-account-sid',
                SSM_TWILIO_AUTH_TOKEN: '/bday-wisher/twilio-auth-token',
                SSM_TWILIO_PHONE_NUMBER: '/bday-wisher/twilio-phone-number',
                TWILIO_WEBHOOK_URL: `${api.url}messages`,
            },
            timeout: Duration.seconds(30),
            memorySize: 512
        });

        dataBucket.grantRead(replyHandler.role!);

        // SSM read access for both Lambdas
        const ssmPolicy = new iam.PolicyStatement({
            actions: ['ssm:GetParameter'],
            resources: [`arn:aws:ssm:${this.region}:${this.account}:parameter/bday-wisher/*`],
        });
        birthdayHandler.addToRolePolicy(ssmPolicy);
        replyHandler.addToRolePolicy(ssmPolicy);

        // Create a resource for the incoming messages
        const messages = api.root.addResource('messages');

        // Create a POST method for the messages resource
        const integration = new apigateway.LambdaIntegration(replyHandler);
        messages.addMethod('POST', integration);

        // Grant API Gateway permission to invoke the Lambda function
        replyHandler.grantInvoke(new iam.ServicePrincipal('apigateway.amazonaws.com'));
    }
}
