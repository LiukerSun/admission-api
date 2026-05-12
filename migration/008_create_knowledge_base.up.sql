CREATE TABLE IF NOT EXISTS knowledge_documents (
    id BIGSERIAL PRIMARY KEY,
    title VARCHAR(500) NOT NULL,
    content TEXT NOT NULL,
    category VARCHAR(50) NOT NULL CHECK (category IN ('policy', 'school', 'major', 'case', 'style', 'general')),
    source VARCHAR(500),
    tags TEXT[],
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_knowledge_documents_category ON knowledge_documents(category);
CREATE INDEX IF NOT EXISTS idx_knowledge_documents_tags ON knowledge_documents USING GIN(tags);

-- Seed some initial knowledge for Heilongjiang Gaokao
INSERT INTO knowledge_documents (title, content, category, tags) VALUES
('黑龙江省新高考模式说明', '黑龙江省自2024年起实施"3+1+2"新高考模式。"3"为语文、数学、外语3门全国统一高考科目；"1"为物理或历史中选择1门作为首选科目；"2"为化学、生物、政治、地理中选择2门作为再选科目。再选科目采用等级赋分制。', 'policy', ARRAY['黑龙江', '新高考', '3+1+2', '赋分']),

('强基计划报考要点', '强基计划主要选拔培养有志于服务国家重大战略需求且综合素质优秀或基础学科拔尖的学生。招生专业聚焦高端芯片与软件、智能科技、新材料、先进制造和国家安全等关键领域以及国家人才紧缺的人文社会科学领域。报考流程：3月底至4月高校发布简章→考生网上报名→6月参加高考→6月下旬高校确定入围名单→7月初高校组织校考→择优录取。', 'policy', ARRAY['强基计划', '招生', '报考流程']),

('提前批院校类型', '提前批主要包括：军事院校、公安院校、司法院校、航海类院校、小语种专业、公费师范生、免费医学生、定向培养军士等。提前批录取在普通本科批之前进行，一旦被提前批录取，不再参加后续批次录取。填报提前批需注意体检、政审、面试等特殊要求。', 'policy', ARRAY['提前批', '军校', '公安', '师范', '报考']),

('冲稳保填报策略', '高考志愿填报建议采用"冲一冲、稳一稳、保一保"的策略。冲：选择往年录取位次略高于自己位次的院校（约高10%-20%），数量占20%-30%；稳：选择往年录取位次与自己位次相当的院校，数量占40%-50%；保：选择往年录取位次明显低于自己位次的院校（约低20%-30%），确保不滑档，数量占20%-30%。注意：具体比例需根据个人风险偏好调整。', 'general', ARRAY['填报策略', '冲稳保', '志愿', '位次']),

('计算机类专业选择建议', '计算机类包括计算机科学与技术、软件工程、信息安全、物联网工程、数据科学与大数据技术等。选择建议：1) 想学底层选计算机科学与技术；2) 想直接就业选软件工程；3) 对安全感兴趣选信息安全；4) 数学好想走算法选数据科学。院校层次很重要，计算机专业建议优先选学科评估B+以上的学校或行业特色院校。', 'major', ARRAY['计算机', '软件工程', '专业选择', '就业']),

('家庭经济一般学生的专业推荐', '如果家庭经济条件一般，希望毕业后尽快就业赚钱，推荐以下方向：1) 工科类：计算机、软件工程、电子信息、电气工程（进电网或企业）；2) 医学类：临床医学（周期长但收入稳定）、口腔医学；3) 师范类：公费师范生（免学费包分配）；4) 警校：公安类专业入警率高。避免选择需要大量家庭资源支撑的专业如金融（名校除外）、艺术类等。', 'style', ARRAY['家庭', '经济', '就业', '赚钱', '推荐'])
ON CONFLICT DO NOTHING;
