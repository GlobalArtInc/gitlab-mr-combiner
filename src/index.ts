import express from 'express';
import bodyParser from 'body-parser';
import { MergeRequest, NoteEvent } from './interfaces';
import { GITLAB_URL, TRIGGER_MESSAGE, TRIGGER_TAG, TARGET_BRANCH, GITLAB_TOKEN, GIT_EMAIL, GIT_USER } from './consts';
import { ApiClient } from './api.client';
import { exec } from 'child_process';
import util from 'util';
import { createLogger, format, transports } from 'winston';

const execPromise = util.promisify(exec);
const PORT = process.env.PORT || 8080;

const logger = createLogger({
  level: 'info',
  format: format.combine(
    format.timestamp(),
    format.printf(({ timestamp, level, message }) => {
      return `${timestamp} [${level}]: ${message}`;
    })
  ),
  transports: [
    new transports.Console()
  ],
});

export class Server {
  private readonly api: ApiClient;
  private readonly app: express.Express;

  constructor() {
    this.api = new ApiClient(GITLAB_URL);
    this.app = express();
  }

  init() {
    this.checkEnvVariables();
    this.initGit();
    this.app.use(bodyParser.json());
    this.app.post('/webhook', this.handleWebhook.bind(this));
    this.app.listen(PORT, async () => {
      await this.initGit();
      logger.info(`Server is listening on port ${PORT}`);
    });
  }

  async initGit() {
    await execPromise(`git config --global user.email "${GIT_EMAIL}"`);
    await execPromise(`git config --global user.name "${GIT_USER}"`);
  }

  private checkEnvVariables() {
    const requiredVars = [
      { name: 'TRIGGER_MESSAGE', value: TRIGGER_MESSAGE },
      { name: 'TRIGGER_TAG', value: TRIGGER_TAG },
      { name: 'TARGET_BRANCH', value: TARGET_BRANCH },
      { name: 'GITLAB_TOKEN', value: GITLAB_TOKEN },
      { name: 'GITLAB_URL', value: GITLAB_URL },
    ];

    requiredVars.forEach(({ name, value }) => {
      if (!value) {
        logger.error(`Environment variable ${name} is missing.`);
        process.exit(1);
      }
    });
  }

  private async handleWebhook(req: express.Request, res: express.Response) {
    const event = req.body as NoteEvent;

    if (this.isTriggerEvent(event)) {
      this.combineAllMRs(event.project_id, event.merge_request.iid);
    }

    res.sendStatus(200);
  }

  private isTriggerEvent(event: NoteEvent): boolean {
    return event.event_type === 'note' && event.object_attributes.action === 'create' && TRIGGER_MESSAGE === event.object_attributes.note;
  }

  private async combineAllMRs(projectId: number, mergeRequestId: number) {
    let logs = '';
    const logMessage = (message: string) => {
      logger.info(message);
      logs += message + '\n';
    };

    try {
      const { defaultBranch, repoUrl } = await this.getRepoInfo(projectId);
      logMessage(`Fetched repo info: branch = ${defaultBranch}, url = ${repoUrl}`);
      
      await this.cloneOrFetchBranch(repoUrl, defaultBranch, projectId, logMessage);
      await this.createBranch(TARGET_BRANCH, defaultBranch, projectId, logMessage);

      const mergeRequests = await this.fetchMergeRequests(projectId);
      logMessage(`Found ${mergeRequests.length} merge requests`);

      await Promise.all(mergeRequests.map((mr: MergeRequest) => this.pullMRToBranch(mr, projectId, logMessage)));

      await this.forcePushToRemote(projectId, logMessage);
      await this.createCommentOnMR(projectId, mergeRequestId, `Merge Requests were rebased into ${TARGET_BRANCH}\n\`\`\`\n${logs}\n\`\`\``);
    } catch (error) {
      logMessage(`Error in combineAllMRs: ${error}`);
      await this.createCommentOnMR(projectId, mergeRequestId, `An error occurred during rebase into ${TARGET_BRANCH}\n\`\`\`\n${logs}\n\`\`\``);
    }
  }

  private async cloneOrFetchBranch(repoUrl: string, defaultBranch: string, projectId: number, logMessage: (msg: string) => void) {
    const clonePath = `/tmp/${projectId}`;
    await execPromise(`rm -rf ${clonePath}`)
    await execPromise(`git clone --branch ${defaultBranch} ${repoUrl} ${clonePath}`);
    logMessage(`Cloned branch ${defaultBranch} to ${clonePath}`);
  }

  private async createBranch(branchName: string, baseBranch: string, projectId: number, logMessage: (msg: string) => void) {
    const clonePath = `/tmp/${projectId}`;

    try {
      await execPromise(`git -C ${clonePath} checkout ${baseBranch}`);
      await execPromise(`git -C ${clonePath} branch -D ${branchName}`);
      logMessage(`Deleted existing branch ${branchName}`);
    } catch (error: any) {
      if (!error.message.includes('did not match any file(s) known to git')) {
        logMessage(`Error deleting branch ${branchName}: ${error.message}`);
      }
    }

    await execPromise(`git -C ${clonePath} checkout -b ${branchName}`);
    logMessage(`Created branch ${branchName}`);
  }

  private async pullMRToBranch(mergeRequest: any, projectId: number, logMessage: (msg: string) => void) {
    const mrId = mergeRequest.iid;
    const clonePath = `/tmp/${projectId}`;
    await execPromise(`cd ${clonePath} && git fetch origin merge-requests/${mrId}/head:mr-${mrId}`);
    await execPromise(`cd ${clonePath} && git checkout ${TARGET_BRANCH}`);
    await execPromise(`cd ${clonePath} && git pull . mr-${mrId} --rebase`);
    
    logMessage(`Merged MR #${mrId} into current branch`);
  }

  private async fetchMergeRequests(projectId: number): Promise<MergeRequest[]> { 
    return this.api.send({
      method: 'GET',
      url: `/projects/${projectId}/merge_requests`,
      params: {
        state: 'opened',
        labels: TRIGGER_TAG,
      },
    });
  }

  private async forcePushToRemote(projectId: number, logMessage: (msg: string) => void) {
    await execPromise(`cd /tmp/${projectId} && git push origin ${TARGET_BRANCH} --force`);
    logMessage(`Force pushed to remote repository`);
  }

  private async getRepoInfo(projectId: number): Promise<{ defaultBranch: string; repoUrl: string }> {
    const project = await this.api.send({
      method: 'GET',
      url: `/projects/${projectId}`,
    });

    return { defaultBranch: project.default_branch, repoUrl: project.ssh_url_to_repo };
  }

  private async createCommentOnMR(projectId: number, mergeRequestId: number, comment: string) {
    await this.api.send({
      method: 'POST',
      url: `/projects/${projectId}/merge_requests/${mergeRequestId}/notes`,
      data: { body: comment },
    });
    logger.info(`Comment added to MR #${mergeRequestId}: ${comment}`);
  }
}

new Server().init();
