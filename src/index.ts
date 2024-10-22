import express from 'express';
import bodyParser from 'body-parser';
import { MergeRequest, NoteEvent } from './interfaces';
import { GITLAB_URL, TRIGGER_MESSAGE, TRIGGER_TAG, TARGET_BRANCH, GITLAB_TOKEN, GIT_EMAIL, GIT_USER } from './consts';
import { ApiClient } from './api.client';
import { exec } from 'child_process';
import util from 'util';

const execPromise = util.promisify(exec);
const PORT = process.env.PORT || 8080;

export class Server {
  private readonly api = new ApiClient(GITLAB_URL);
  private readonly app = express();

  init() {
    this.checkEnvVariables();
    this.app.use(bodyParser.json());
    this.app.post('/webhook', this.handleWebhook.bind(this));
    this.app.listen(PORT, () => console.log(`Server is listening on port ${PORT}`));
  }

  checkEnvVariables() {
    const requiredVars = [
      { name: 'TRIGGER_MESSAGE', value: TRIGGER_MESSAGE },
      { name: 'TRIGGER_TAG', value: TRIGGER_TAG },
      { name: 'TARGET_BRANCH', value: TARGET_BRANCH },
      { name: 'GITLAB_TOKEN', value: GITLAB_TOKEN },
      { name: 'GITLAB_URL', value: GITLAB_URL },
    ];

    requiredVars.forEach(({ name, value }) => {
      if (!value) {
        console.error(`Environment variable ${name} is missing.`);
        process.exit(1);
      }
    });
  }

  async handleWebhook(req: express.Request, res: express.Response) {
    const event = req.body as NoteEvent;

    if (this.isTriggerEvent(event)) {
      await this.combineAllMRs(event.project_id, event.merge_request.iid);
    }

    res.sendStatus(200);
  }

  isTriggerEvent(event: NoteEvent): boolean {
    return event.event_type === 'note' && event.object_attributes.action === 'create' && TRIGGER_MESSAGE === event.object_attributes.note;
  }

  async combineAllMRs(projectId: number, mergeRequestId: number) {
    try {
      const { defaultBranch, repoUrl } = await this.getRepoInfo(projectId);
      await this.cloneOrFetchBranch(repoUrl, defaultBranch, projectId);
      await this.createBranch(TARGET_BRANCH, defaultBranch, projectId);

      const mergeRequests = await this.fetchMergeRequests(projectId);
      await Promise.all(mergeRequests.map((mr: MergeRequest) => this.mergeMRToBranch(mr, projectId)));

      await this.forcePushToRemote(projectId);
      await this.createCommentOnMR(projectId, mergeRequestId, `Merge Requests were rebased into ${TARGET_BRANCH}`);
    } catch (error) {
      console.error(`Error in combineAllMRs: ${error}`);
      await this.createCommentOnMR(projectId, mergeRequestId, `An error occurred during rebase into ${TARGET_BRANCH}`);
    }
  }

  async cloneOrFetchBranch(repoUrl: string, defaultBranch: string, projectId: number) {
    const clonePath = `/tmp/${projectId}`;
    execPromise(`git config --global user.email "${GIT_EMAIL}"`);
    execPromise(`git config --global user.name "${GIT_USER}"`);

    try {
      await execPromise(`git -C ${clonePath} fetch`);
      await execPromise(`git -C ${clonePath} reset --hard origin/${defaultBranch}`);
      console.log(`Updated existing branch ${defaultBranch} in ${clonePath}`);
    } catch {
      await execPromise(`git clone --single-branch --branch ${defaultBranch} ${repoUrl} ${clonePath}`);
      console.log(`Cloned branch ${defaultBranch} to ${clonePath}`);
    }
  }

  async createBranch(branchName: string, baseBranch: string, projectId: number) {
    const clonePath = `/tmp/${projectId}`;

    try {
      await execPromise(`git -C ${clonePath} checkout ${baseBranch}`);
      await execPromise(`git -C ${clonePath} branch -D ${branchName}`);
      console.log(`Deleted existing branch ${branchName}`);
    } catch (error: any) {
      if (!error.message.includes('did not match any file(s) known to git')) {
        console.log(`Error deleting branch ${branchName}: ${error.message}`);
      }
    }

    await execPromise(`git -C ${clonePath} checkout -b ${branchName}`);
    console.log(`Created branch ${branchName}`);
  }

  async fetchMergeRequests(projectId: number) {
    return this.api.send({
      method: 'GET',
      url: `/projects/${projectId}/merge_requests`,
      params: {
        state: 'opened',
        labels: TRIGGER_TAG,
      },
    });
  }

  async mergeMRToBranch(mergeRequest: any, projectId: number) {
    const mrId = mergeRequest.iid;
    await execPromise(`cd /tmp/${projectId} && git fetch origin merge-requests/${mrId}/head:mr-${mrId}`);
    await execPromise(`cd /tmp/${projectId} && git merge mr-${mrId}`);
    console.log(`Merged MR #${mrId} into current branch`);
  }

  async forcePushToRemote(projectId: number) {
    await execPromise(`cd /tmp/${projectId} && git push origin ${TARGET_BRANCH} --force`);
    console.log(`Force pushed to remote repository`);
  }

  async getRepoInfo(projectId: number): Promise<{ defaultBranch: string; repoUrl: string }> {
    const project = await this.api.send({
      method: 'GET',
      url: `/projects/${projectId}`,
    });

    return { defaultBranch: project.default_branch, repoUrl: project.ssh_url_to_repo };
  }

  async createCommentOnMR(projectId: number, mergeRequestId: number, comment: string) {
    await this.api.send({
      method: 'POST',
      url: `/projects/${projectId}/merge_requests/${mergeRequestId}/notes`,
      data: { body: comment },
    });
    console.log(`Comment added to MR #${mergeRequestId}: ${comment}`);
  }
}

new Server().init();
