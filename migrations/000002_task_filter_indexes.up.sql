CREATE INDEX idx_tasks_team_status ON tasks (team_id, status);
CREATE INDEX idx_tasks_team_assignee ON tasks (team_id, assignee_id);
