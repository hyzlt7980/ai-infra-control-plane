CREATE TABLE IF NOT EXISTS models (
  model_name VARCHAR(191) NOT NULL,
  model_type VARCHAR(64) NOT NULL,
  version VARCHAR(64) NOT NULL,
  image VARCHAR(255) NOT NULL,
  container_port INT NOT NULL,
  status VARCHAR(32) NOT NULL,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (model_name),
  INDEX idx_models_model_type_status (model_type, status)
);

CREATE TABLE IF NOT EXISTS history_records (
  request_id VARCHAR(191) NOT NULL,
  model_name VARCHAR(191) NOT NULL,
  model_type VARCHAR(64) NOT NULL,
  status VARCHAR(32) NOT NULL,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  summary TEXT NOT NULL,
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (request_id),
  INDEX idx_history_model_created_at (model_name, created_at),
  INDEX idx_history_status_created_at (status, created_at)
);
