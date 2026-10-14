INSERT INTO users (username, password_hash, full_name, role, active) VALUES
  ('admin', '$2a$10$.B83I1kn7YMISyO0NRuDaOHlhseLOctjchER12FSgaJRyfVjiDVpa', 'Administrator', 'admin', TRUE),
  ('staff', '$2a$10$9E/JBsYbX.onRjbScHVi/.8jxABmef3ragiMJAjpknpCJO.iSSjG6', 'Staff Member',  'staff', TRUE)
ON CONFLICT (username) DO NOTHING;
