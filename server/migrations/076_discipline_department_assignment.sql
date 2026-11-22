-- Keep the saved action selection when temporarily turning off responsibility.
ALTER TABLE departments ADD COLUMN discipline_enabled boolean NOT NULL DEFAULT false;
UPDATE departments SET discipline_enabled=true WHERE cardinality(discipline_scope)>0;
