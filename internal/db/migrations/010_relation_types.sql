-- +goose Up
CREATE TABLE relation_types (
    name TEXT PRIMARY KEY,
    description TEXT NOT NULL,
    tenant_id UUID,
    created_at TIMESTAMPTZ DEFAULT now()
);

-- Seed canonical types (global: tenant_id is NULL).
INSERT INTO relation_types (name, description) VALUES
('created', 'A built/founded/authored B'),
('founded', 'A founded/established B'),
('works_at', 'A is employed at B'),
('worked_at', 'A was formerly employed at B'),
('works_on', 'A is actively working on B'),
('leads', 'A leads/manages B'),
('owns', 'A owns/possesses B'),
('part_of', 'A is part of B'),
('product_of', 'A is a product of B'),
('deployed_on', 'A is deployed on B'),
('runs_on', 'A runs on B'),
('uses', 'A uses/utilizes B'),
('depends_on', 'A depends on B'),
('implements', 'A implements B'),
('extends', 'A extends/inherits from B'),
('replaced_by', 'A was replaced by B'),
('enables', 'A enables/powers B'),
('supports', 'A supports B'),
('parent_of', 'A is the parent of B'),
('child_of', 'A is the child of B'),
('sibling_of', 'A is a sibling of B'),
('married_to', 'A is married to B'),
('friend_of', 'A is a friend of B'),
('mentored', 'A mentored B'),
('located_in', 'A is located in B'),
('learned', 'A learned B'),
('decided', 'A decided B'),
('inspired', 'A was inspired by B'),
('prefers', 'A prefers B'),
('competes_with', 'A competes with B'),
('acquired', 'A acquired B'),
('funded', 'A funded B'),
('partners_with', 'A partners with B'),
('affected_by', 'A was affected by B'),
('achieved', 'A achieved B'),
('detected_in', 'A was detected in B'),
('experienced', 'A experienced B')
ON CONFLICT DO NOTHING;

-- +goose Down
DROP TABLE IF EXISTS relation_types;
