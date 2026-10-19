INSERT INTO users (username, password_hash, full_name, role, active) VALUES
  ('jm', '$2a$10$oZcnBugdDmuONkO3XHzu8.zkRppqmlYJO1jrs5cE7TvsVBZd1mYqC', 'JM', 'admin', TRUE)
ON CONFLICT (username) DO NOTHING;