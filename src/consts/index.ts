export const TRIGGER_MESSAGE = process.env.TRIGGER_MESSAGE;
export const TRIGGER_TAG = process.env.TRIGGER_TAG;
export const TARGET_BRANCH = process.env.TARGET_BRANCH as string;
export const GITLAB_TOKEN = process.env.GITLAB_TOKEN;
export const GITLAB_URL = process.env.GITLAB_URL || 'https://gitlab.com/';
export const GIT_EMAIL = process.env.GIT_EMAIL || 'vcs@example.com';
export const GIT_USER = process.env.GIT_USER || 'vcs';