#!/usr/bin/env node
import 'source-map-support/register';
import * as cdk from 'aws-cdk-lib';
import { BirthdayWisherStack } from '../lib/birthday-wisher-stack';

const app = new cdk.App();
new BirthdayWisherStack(app, 'BirthdayWisherStack');

