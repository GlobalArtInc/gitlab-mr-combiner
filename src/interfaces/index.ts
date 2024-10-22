interface User {
  id: number;
  name: string;
  username: string;
  avatar_url: string;
  email: string;
}

interface Project {
  id: number;
  name: string;
  description: string;
  web_url: string;
  avatar_url: string;
  git_ssh_url: string;
  git_http_url: string;
  namespace: string;
  visibility_level: number;
  path_with_namespace: string;
  default_branch: string;
  ci_config_path: string | null;
  homepage: string;
  url: string;
  ssh_url: string;
  http_url: string;
}

interface ObjectAttributes {
  attachment: null;
  author_id: number;
  change_position: null;
  commit_id: null;
  created_at: string;
  discussion_id: string;
  id: number;
  line_code: null;
  note: string;
  noteable_id: number;
  noteable_type: string;
  original_position: null;
  position: null;
  project_id: number;
  resolved_at: null;
  resolved_by_id: null;
  resolved_by_push: null;
  st_diff: null;
  system: boolean;
  type: null;
  updated_at: string;
  updated_by_id: null;
  description: string;
  url: string;
  action: string;
}

interface Repository {
  name: string;
  url: string;
  description: string;
  homepage: string;
}

export interface MergeRequest {
  assignee_id: null;
  author_id: number;
  created_at: string;
  description: string;
  draft: boolean;
  head_pipeline_id: null;
  id: number;
  iid: number;
  last_edited_at: null;
  last_edited_by_id: null;
  merge_commit_sha: null;
  merge_error: null;
  merge_params: {
      force_remove_source_branch: boolean;
  };
  merge_status: string;
  merge_user_id: null;
  merge_when_pipeline_succeeds: boolean;
  milestone_id: null;
  source_branch: string;
  source_project_id: number;
  state_id: number;
  target_branch: string;
  target_project_id: number;
  time_estimate: number;
  title: string;
  updated_at: string;
  updated_by_id: null;
  prepared_at: string;
  assignee_ids: number[];
  blocking_discussions_resolved: boolean;
  detailed_merge_status: string;
  first_contribution: boolean;
  labels: Array<{
      id: number;
      title: string;
      color: string;
      project_id: number;
      created_at: string;
      updated_at: string;
      template: boolean;
      description: string | null;
      type: string;
      group_id: null;
  }>;
  last_commit: {
      id: string;
      message: string;
      title: string;
      timestamp: string;
      url: string;
      author: {
          name: string;
      };
  };
}

export interface NoteEvent {
  object_kind: string;
  event_type: string;
  user: User;
  project_id: number;
  project: Project;
  object_attributes: ObjectAttributes;
  repository: Repository;
  merge_request: MergeRequest;
}