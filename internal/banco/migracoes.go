package banco

// migracoes é a história do esquema. Uma versão publicada nunca é editada:
// mudanças entram como um novo item no fim da lista.
//
// Datas são guardadas em milissegundos desde 1970 (UTC).
var migracoes = []string{
	// 1: esquema inicial.
	`
CREATE TABLE atividades (
	id            INTEGER PRIMARY KEY AUTOINCREMENT,
	tipo          TEXT NOT NULL,
	titulo        TEXT NOT NULL,
	objetivo      TEXT NOT NULL,
	estado        TEXT NOT NULL,
	aviso         TEXT NOT NULL DEFAULT '',
	orcamento_usd REAL NOT NULL,
	dados         TEXT NOT NULL DEFAULT '{}',
	criada_em     INTEGER NOT NULL,
	atualizada_em INTEGER NOT NULL
);

CREATE TABLE eventos (
	id           INTEGER PRIMARY KEY AUTOINCREMENT,
	atividade_id INTEGER NOT NULL REFERENCES atividades(id) ON DELETE CASCADE,
	quando       INTEGER NOT NULL,
	origem       TEXT NOT NULL,
	tipo         TEXT NOT NULL,
	texto        TEXT NOT NULL,
	dados        TEXT NOT NULL DEFAULT '{}'
);
CREATE INDEX eventos_atividade ON eventos(atividade_id, id);

CREATE TABLE gastos (
	id           INTEGER PRIMARY KEY AUTOINCREMENT,
	quando       INTEGER NOT NULL,
	atividade_id INTEGER NOT NULL DEFAULT 0,
	modelo       TEXT NOT NULL,
	finalidade   TEXT NOT NULL,
	tokens_in    INTEGER NOT NULL,
	tokens_out   INTEGER NOT NULL,
	custo_usd    REAL NOT NULL
);
CREATE INDEX gastos_atividade ON gastos(atividade_id);
CREATE INDEX gastos_quando ON gastos(quando);

CREATE TABLE confirmacoes (
	id           INTEGER PRIMARY KEY AUTOINCREMENT,
	atividade_id INTEGER NOT NULL REFERENCES atividades(id) ON DELETE CASCADE,
	ferramenta   TEXT NOT NULL,
	argumentos   TEXT NOT NULL,
	motivos      TEXT NOT NULL,
	estado       TEXT NOT NULL,
	criada_em    INTEGER NOT NULL,
	decidida_em  INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE documentos (
	id            INTEGER PRIMARY KEY AUTOINCREMENT,
	sha256        TEXT NOT NULL UNIQUE,
	nome          TEXT NOT NULL,
	tipo          TEXT NOT NULL,
	tamanho       INTEGER NOT NULL,
	origem        TEXT NOT NULL,
	importado_em  INTEGER NOT NULL,
	estado        TEXT NOT NULL,
	erro          TEXT NOT NULL DEFAULT '',
	resumo        TEXT NOT NULL DEFAULT '',
	paginas       INTEGER NOT NULL DEFAULT 0,
	enriquecido   INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE trechos (
	id           INTEGER PRIMARY KEY AUTOINCREMENT,
	documento_id INTEGER NOT NULL REFERENCES documentos(id) ON DELETE CASCADE,
	pagina       INTEGER NOT NULL,
	ordem        INTEGER NOT NULL,
	texto        TEXT NOT NULL
);
CREATE INDEX trechos_documento ON trechos(documento_id, ordem);

CREATE VIRTUAL TABLE trechos_fts USING fts5(
	texto, content='trechos', content_rowid='id',
	tokenize='unicode61 remove_diacritics 2'
);
CREATE TRIGGER trechos_ai AFTER INSERT ON trechos BEGIN
	INSERT INTO trechos_fts(rowid, texto) VALUES (new.id, new.texto);
END;
CREATE TRIGGER trechos_ad AFTER DELETE ON trechos BEGIN
	INSERT INTO trechos_fts(trechos_fts, rowid, texto) VALUES ('delete', old.id, old.texto);
END;

CREATE TABLE vetores (
	trecho_id INTEGER PRIMARY KEY REFERENCES trechos(id) ON DELETE CASCADE,
	modelo    TEXT NOT NULL,
	vetor     BLOB NOT NULL
);

CREATE TABLE entidades (
	id           INTEGER PRIMARY KEY AUTOINCREMENT,
	documento_id INTEGER NOT NULL REFERENCES documentos(id) ON DELETE CASCADE,
	tipo         TEXT NOT NULL,
	valor        TEXT NOT NULL,
	UNIQUE(documento_id, tipo, valor)
);
CREATE INDEX entidades_valor ON entidades(tipo, valor);

CREATE TABLE colecoes (
	id         INTEGER PRIMARY KEY AUTOINCREMENT,
	nome       TEXT NOT NULL UNIQUE,
	criada_por TEXT NOT NULL,
	criada_em  INTEGER NOT NULL
);

CREATE TABLE colecao_documentos (
	colecao_id   INTEGER NOT NULL REFERENCES colecoes(id) ON DELETE CASCADE,
	documento_id INTEGER NOT NULL REFERENCES documentos(id) ON DELETE CASCADE,
	PRIMARY KEY (colecao_id, documento_id)
);

CREATE TABLE entrada_vistos (
	caminho    TEXT PRIMARY KEY,
	tamanho    INTEGER NOT NULL,
	modificado INTEGER NOT NULL
);

CREATE TABLE preferencias_email (
	remetente     TEXT PRIMARY KEY,
	classe        TEXT NOT NULL,
	assunto       TEXT NOT NULL DEFAULT '',
	atualizada_em INTEGER NOT NULL
);

CREATE TABLE cartoes (
	id            INTEGER PRIMARY KEY AUTOINCREMENT,
	atividade_id  INTEGER NOT NULL REFERENCES atividades(id) ON DELETE CASCADE,
	assunto       TEXT NOT NULL,
	frente        TEXT NOT NULL,
	verso         TEXT NOT NULL,
	fonte         TEXT NOT NULL,
	estabilidade  REAL NOT NULL DEFAULT 0,
	dificuldade   REAL NOT NULL DEFAULT 0,
	repeticoes    INTEGER NOT NULL DEFAULT 0,
	lapsos        INTEGER NOT NULL DEFAULT 0,
	revisado_em   INTEGER NOT NULL DEFAULT 0,
	proxima_em    INTEGER NOT NULL,
	UNIQUE(atividade_id, frente)
);
CREATE INDEX cartoes_proxima ON cartoes(atividade_id, proxima_em);

CREATE TABLE questoes (
	id           INTEGER PRIMARY KEY AUTOINCREMENT,
	atividade_id INTEGER NOT NULL REFERENCES atividades(id) ON DELETE CASCADE,
	assunto      TEXT NOT NULL,
	enunciado    TEXT NOT NULL,
	alternativas TEXT NOT NULL,
	correta      INTEGER NOT NULL,
	explicacao   TEXT NOT NULL,
	fonte        TEXT NOT NULL,
	criada_em    INTEGER NOT NULL,
	UNIQUE(atividade_id, enunciado)
);

CREATE TABLE respostas (
	id           INTEGER PRIMARY KEY AUTOINCREMENT,
	atividade_id INTEGER NOT NULL REFERENCES atividades(id) ON DELETE CASCADE,
	assunto      TEXT NOT NULL,
	tipo         TEXT NOT NULL,
	item_id      INTEGER NOT NULL,
	nota         INTEGER NOT NULL,
	correta      INTEGER NOT NULL,
	quando       INTEGER NOT NULL
);
CREATE INDEX respostas_atividade ON respostas(atividade_id, assunto);

CREATE TABLE cache_modelos (
	chave     TEXT PRIMARY KEY,
	resposta  TEXT NOT NULL,
	criada_em INTEGER NOT NULL
);
`,
}
