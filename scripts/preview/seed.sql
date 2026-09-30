-- Conversations for the panel preview. Times are relative to now.
DELETE FROM messages; DELETE FROM chats; DELETE FROM contacts; DELETE FROM groups; DELETE FROM group_participants;
INSERT INTO contacts (jid, phone, push_name, full_name, updated_at) VALUES
 ('5511987654321@s.whatsapp.net','5511987654321','Fulano','Fulano de Tal',strftime('%s','now')),
 ('5511912340001@s.whatsapp.net','5511912340001','Mãe','Mãe',strftime('%s','now')),
 ('5511912340002@s.whatsapp.net','5511912340002','Lucas','Lucas Almeida',strftime('%s','now')),
 ('5511912340003@s.whatsapp.net','5511912340003','Ana','Ana Beatriz',strftime('%s','now')),
 ('5511912340004@s.whatsapp.net','5511912340004','Rafa','Rafael Costa',strftime('%s','now')),
 ('5511912340005@s.whatsapp.net','5511912340005','Dra. Paula','Dra. Paula Mendes',strftime('%s','now')),
 ('5511912340006@s.whatsapp.net','5511912340006','João','João da Silva',strftime('%s','now')),
 ('5511912340007@s.whatsapp.net','5511912340007','Carla','Carla Nunes',strftime('%s','now')),
 ('5511912340008@s.whatsapp.net','5511912340008','Pedro','Pedro Henrique',strftime('%s','now'));
INSERT INTO chats (jid, kind, name, last_message_ts, pinned, unread_count) VALUES
 ('5511912340001@s.whatsapp.net','dm','Mãe',strftime('%s','now')-240,1,2),
 ('120363000000000001@g.us','group','Família Orlandi',strftime('%s','now')-900,0,14),
 ('5511912340002@s.whatsapp.net','dm','Lucas Almeida',strftime('%s','now')-2400,0,0),
 ('120363000000000002@g.us','group','Time de Produto',strftime('%s','now')-5400,0,3),
 ('5511912340003@s.whatsapp.net','dm','Ana Beatriz',strftime('%s','now')-9000,0,1),
 ('5511912340005@s.whatsapp.net','dm','Dra. Paula Mendes',strftime('%s','now')-30000,0,0),
 ('5511912340006@s.whatsapp.net','dm','João da Silva',strftime('%s','now')-610000,0,0),
 ('120363000000000003@g.us','group','Churrasco sexta 🍖',strftime('%s','now')-620000,0,0),
 ('5511912340007@s.whatsapp.net','dm','Carla Nunes',strftime('%s','now')-650000,0,0),
 ('5511912340008@s.whatsapp.net','dm','Pedro Henrique',strftime('%s','now')-680000,0,0),
 ('5511912340004@s.whatsapp.net','dm','Rafael Costa',strftime('%s','now')-700000,0,0);
INSERT INTO groups (jid, name, owner_jid, created_ts, updated_at) VALUES
 ('120363000000000001@g.us','Família Orlandi','5511912340001@s.whatsapp.net',strftime('%s','now')-99999999,strftime('%s','now')),
 ('120363000000000002@g.us','Time de Produto','5511987654321@s.whatsapp.net',strftime('%s','now')-9999999,strftime('%s','now')),
 ('120363000000000003@g.us','Churrasco sexta 🍖','5511912340004@s.whatsapp.net',strftime('%s','now')-999999,strftime('%s','now'));
-- helper: (chat, id, sender, secs ago, from_me, text, media)
INSERT INTO messages (chat_jid, chat_name, msg_id, sender_jid, sender_name, ts, from_me, text, display_text, media_type) VALUES
 ('5511912340001@s.whatsapp.net','Mãe','M1','5511912340001@s.whatsapp.net','Mãe',strftime('%s','now')-4000,1,'Oi mãe, cheguei bem!','Oi mãe, cheguei bem!',NULL),
 ('5511912340001@s.whatsapp.net','Mãe','M2','5511912340001@s.whatsapp.net','Mãe',strftime('%s','now')-3000,0,'Que bom filho! Almoça aqui domingo?','Que bom filho! Almoça aqui domingo?',NULL),
 ('5511912340001@s.whatsapp.net','Mãe','M3','5511987654321@s.whatsapp.net','Fulano',strftime('%s','now')-2000,1,'Almoço sim 😊','Almoço sim 😊',NULL),
 ('5511912340001@s.whatsapp.net','Mãe','M4','5511912340001@s.whatsapp.net','Mãe',strftime('%s','now')-300,0,'[Audio]','[Audio]','audio'),
 ('5511912340001@s.whatsapp.net','Mãe','M5','5511912340001@s.whatsapp.net','Mãe',strftime('%s','now')-240,0,'Traz a sobremesa então!','Traz a sobremesa então!',NULL),
 ('120363000000000001@g.us','Família Orlandi','F1','5511912340003@s.whatsapp.net','Ana',strftime('%s','now')-3600,0,'Alguém vai no aniversário da vó?','Alguém vai no aniversário da vó?',NULL),
 ('120363000000000001@g.us','Família Orlandi','F2','5511912340002@s.whatsapp.net','Lucas',strftime('%s','now')-2500,0,'Eu vou! Levo o bolo','Eu vou! Levo o bolo',NULL),
 ('120363000000000001@g.us','Família Orlandi','F3','5511912340001@s.whatsapp.net','Mãe',strftime('%s','now')-900,0,'Foto do bolo do ano passado kkk','Foto do bolo do ano passado kkk','image'),
 ('5511912340002@s.whatsapp.net','Lucas Almeida','L1','5511987654321@s.whatsapp.net','Fulano',strftime('%s','now')-5000,1,'Bom dia! Confirma nossa call das 14h?','Bom dia! Confirma nossa call das 14h?',NULL),
 ('5511912340002@s.whatsapp.net','Lucas Almeida','L2','5511912340002@s.whatsapp.net','Lucas',strftime('%s','now')-2400,0,'Confirmado! Te mando o link','Confirmado! Te mando o link',NULL),
 ('120363000000000002@g.us','Time de Produto','P1','5511912340007@s.whatsapp.net','Carla',strftime('%s','now')-7200,0,'Subi a versão nova no staging','Subi a versão nova no staging',NULL),
 ('120363000000000002@g.us','Time de Produto','P2','5511912340008@s.whatsapp.net','Pedro',strftime('%s','now')-5400,0,'Testando agora, parece tudo ok 👍','Testando agora, parece tudo ok 👍',NULL),
 ('5511912340003@s.whatsapp.net','Ana Beatriz','A1','5511912340003@s.whatsapp.net','Ana',strftime('%s','now')-9000,0,'Você viu o contrato que eu mandei?','Você viu o contrato que eu mandei?',NULL),
 ('5511912340005@s.whatsapp.net','Dra. Paula Mendes','D1','5511912340005@s.whatsapp.net','Dra. Paula',strftime('%s','now')-30000,0,'Sua consulta está confirmada para quinta, 10h.','Sua consulta está confirmada para quinta, 10h.',NULL),
 ('5511912340006@s.whatsapp.net','João da Silva','J1','5511987654321@s.whatsapp.net','Fulano',strftime('%s','now')-610000,1,'Proposta_final.pdf','Proposta_final.pdf','document'),
 ('120363000000000003@g.us','Churrasco sexta 🍖','C1','5511912340004@s.whatsapp.net','Rafa',strftime('%s','now')-620000,0,'Quem leva o carvão?','Quem leva o carvão?',NULL),
 ('5511912340007@s.whatsapp.net','Carla Nunes','K1','5511912340007@s.whatsapp.net','Carla',strftime('%s','now')-650000,0,'Obrigada pela ajuda ontem!','Obrigada pela ajuda ontem!',NULL),
 ('5511912340008@s.whatsapp.net','Pedro Henrique','H1','5511987654321@s.whatsapp.net','Fulano',strftime('%s','now')-680000,1,'Valeu, até semana que vem','Valeu, até semana que vem',NULL),
 ('5511912340004@s.whatsapp.net','Rafael Costa','R1','5511912340004@s.whatsapp.net','Rafa',strftime('%s','now')-700000,0,'Bora marcar aquele futebol','Bora marcar aquele futebol',NULL);
