INSERT INTO dictionary_types(id,code,name,description,system,enabled) VALUES
('30000000-0000-0000-0000-000000000020','DISCIPLINE_LEVEL','处分档位','处分规则使用的标准档位名称',true,true)
ON CONFLICT (code) DO NOTHING;

INSERT INTO dictionary_items(id,type_id,value,label,description,sort_order,enabled) VALUES
('31000000-0000-0000-0000-000000000020',(SELECT id FROM dictionary_types WHERE code='DISCIPLINE_LEVEL'),'NOTICE','通报批评','',10,true),
('31000000-0000-0000-0000-000000000021',(SELECT id FROM dictionary_types WHERE code='DISCIPLINE_LEVEL'),'WARNING','警告','',20,true),
('31000000-0000-0000-0000-000000000022',(SELECT id FROM dictionary_types WHERE code='DISCIPLINE_LEVEL'),'SERIOUS_WARNING','严重警告','',30,true),
('31000000-0000-0000-0000-000000000023',(SELECT id FROM dictionary_types WHERE code='DISCIPLINE_LEVEL'),'DEMERIT','记过','',40,true),
('31000000-0000-0000-0000-000000000024',(SELECT id FROM dictionary_types WHERE code='DISCIPLINE_LEVEL'),'PROBATION','留校察看','',50,true)
ON CONFLICT (type_id,value) DO NOTHING;
