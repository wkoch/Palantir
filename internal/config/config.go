// Package config lê a configuração do Palantír e define onde ficam os
// arquivos do sistema.
package config

import (
	"bytes"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

//go:embed padrao.toml
var padrao []byte

// Padrao devolve o conteúdo do arquivo de configuração inicial.
func Padrao() []byte { return bytes.Clone(padrao) }

// Caminhos reúne os diretórios usados pelo daemon.
type Caminhos struct {
	Config   string // diretório com config.toml e ajustes.toml
	Dados    string // banco, originais e diário
	Execucao string // sockets e arquivos temporários
}

// CaminhosPadrao segue a especificação XDG. Se raiz não for vazia, tudo fica
// abaixo dela, o que isola testes e ambientes de desenvolvimento.
func CaminhosPadrao(raiz string) (Caminhos, error) {
	if raiz != "" {
		abs, err := filepath.Abs(raiz)
		if err != nil {
			return Caminhos{}, err
		}
		return Caminhos{
			Config:   filepath.Join(abs, "config"),
			Dados:    filepath.Join(abs, "dados"),
			Execucao: execucaoDe(abs),
		}, nil
	}
	casa, err := os.UserHomeDir()
	if err != nil {
		return Caminhos{}, err
	}
	base := func(variavel, reserva string) string {
		if v := os.Getenv(variavel); filepath.IsAbs(v) {
			return filepath.Join(v, "palantir")
		}
		return filepath.Join(reserva, "palantir")
	}
	dados := base("XDG_DATA_HOME", filepath.Join(casa, ".local", "share"))
	return Caminhos{
		Config:   base("XDG_CONFIG_HOME", filepath.Join(casa, ".config")),
		Dados:    dados,
		Execucao: base("XDG_RUNTIME_DIR", filepath.Join(dados, "run")),
	}, nil
}

// limiteDoSocket deixa folga abaixo dos 108 bytes de um caminho de socket Unix.
const limiteDoSocket = 100

// execucaoDe usa a pasta "run" dentro da raiz, ou uma pasta curta no diretório
// de execução quando o caminho da raiz não cabe num socket.
func execucaoDe(raiz string) string {
	dentro := filepath.Join(raiz, "run")
	if len(filepath.Join(dentro, "proxy-extrair", "proxy.sock")) <= limiteDoSocket {
		return dentro
	}
	base := os.Getenv("XDG_RUNTIME_DIR")
	if !filepath.IsAbs(base) {
		base = os.TempDir()
	}
	soma := sha256.Sum256([]byte(raiz))
	return filepath.Join(base, "palantir-"+hex.EncodeToString(soma[:6]))
}

// Criar garante que os diretórios existam com acesso só para o dono.
func (c Caminhos) Criar() error {
	for _, d := range []string{c.Config, c.Dados, c.Execucao, c.Originais()} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			return fmt.Errorf("criar %s: %w", d, err)
		}
	}
	return nil
}

// Arquivo é o caminho do config.toml.
func (c Caminhos) Arquivo() string { return filepath.Join(c.Config, "config.toml") }

// Ajustes é o caminho do arquivo gravado pela interface.
func (c Caminhos) Ajustes() string { return filepath.Join(c.Config, "ajustes.toml") }

// Segredos é o arquivo do armazenamento de segredos em disco.
func (c Caminhos) Segredos() string { return filepath.Join(c.Config, "segredos.json") }

// Banco é o arquivo SQLite.
func (c Caminhos) Banco() string { return filepath.Join(c.Dados, "palantir.db") }

// Originais é o diretório dos originais imutáveis.
func (c Caminhos) Originais() string { return filepath.Join(c.Dados, "originais") }

// Diario é o arquivo do diário de auditoria.
func (c Caminhos) Diario() string { return filepath.Join(c.Dados, "diario.jsonl") }

// Config é a configuração completa do daemon.
type Config struct {
	Servidor    Servidor    `toml:"servidor" json:"servidor"`
	Modelos     Modelos     `toml:"modelos" json:"modelos"`
	Orcamento   Orcamento   `toml:"orcamento" json:"orcamento"`
	Ferramentas Ferramentas `toml:"ferramentas" json:"ferramentas"`
	IMAP        IMAP        `toml:"imap" json:"imap"`
	Acervo      Acervo      `toml:"acervo" json:"acervo"`
	Segredos    Segredos    `toml:"segredos" json:"segredos"`
	Busca       Busca       `toml:"busca" json:"busca"`
}

// Busca configura o mecanismo de pesquisa usado pela IA, um SearXNG.
type Busca struct {
	// URL é a base do SearXNG; vazia desliga a pesquisa.
	URL string `toml:"url" json:"url"`
}

// Servidor configura a interface HTTP local.
type Servidor struct {
	Endereco string `toml:"endereco" json:"endereco"`
}

// Modelos configura o roteador de modelos.
type Modelos struct {
	URL        string `toml:"url" json:"url"`
	Barato     string `toml:"barato" json:"barato"`
	Principal  string `toml:"principal" json:"principal"`
	Forte      string `toml:"forte" json:"forte"`
	Embeddings string `toml:"embeddings" json:"embeddings"`
	// Decisoes é o modelo de respostas fechadas, usado na triagem e no juiz.
	Decisoes   string `toml:"decisoes" json:"decisoes"`
	Juiz       bool   `toml:"juiz" json:"juiz"`
	Enriquecer bool   `toml:"enriquecer" json:"enriquecer"`
}

// Orcamento limita o gasto em dólares.
type Orcamento struct {
	MensalUSD       float64 `toml:"mensal_usd" json:"mensal_usd"`
	PorAtividadeUSD float64 `toml:"por_atividade_usd" json:"por_atividade_usd"`
}

// Ferramentas configura a execução das ferramentas MCP.
type Ferramentas struct {
	Isolamento              string `toml:"isolamento" json:"isolamento"`
	Imagem                  string `toml:"imagem" json:"imagem"`
	Binario                 string `toml:"binario" json:"binario"`
	PermitirEnderecosLocais bool   `toml:"permitir_enderecos_locais" json:"permitir_enderecos_locais"`
	Web                     Web    `toml:"web" json:"web"`
}

// Web lista os hosts liberados para a pesquisa.
type Web struct {
	Hosts []string `toml:"hosts" json:"hosts"`
}

// IMAP configura a caixa de e-mail da triagem.
type IMAP struct {
	Servidor  string `toml:"servidor" json:"servidor"`
	Porta     int    `toml:"porta" json:"porta"`
	Usuario   string `toml:"usuario" json:"usuario"`
	Caixa     string `toml:"caixa" json:"caixa"`
	Limite    int    `toml:"limite" json:"limite"`
	Etiquetar bool   `toml:"etiquetar" json:"etiquetar"`
	Seguranca string `toml:"seguranca" json:"seguranca"`
}

// Acervo configura a entrada de documentos.
type Acervo struct {
	PastaEntrada      string `toml:"pasta_entrada" json:"pasta_entrada"`
	IntervaloSegundos int    `toml:"intervalo_segundos" json:"intervalo_segundos"`
}

// Segredos escolhe onde as chaves são guardadas.
type Segredos struct {
	Armazenamento string `toml:"armazenamento" json:"armazenamento"`
}

// Carregar lê o padrão embutido, o config.toml e o ajustes.toml, nessa ordem.
// O config.toml é criado na primeira execução.
func Carregar(c Caminhos) (Config, error) {
	var cfg Config
	if _, err := toml.Decode(string(padrao), &cfg); err != nil {
		return cfg, fmt.Errorf("configuração padrão: %w", err)
	}
	if _, err := os.Stat(c.Arquivo()); errors.Is(err, fs.ErrNotExist) {
		if err := os.MkdirAll(c.Config, 0o700); err != nil {
			return cfg, err
		}
		if err := criarInteiro(c.Arquivo(), padrao); err != nil && !errors.Is(err, fs.ErrExist) {
			return cfg, fmt.Errorf("criar configuração: %w", err)
		}
	}
	for _, arquivo := range []string{c.Arquivo(), c.Ajustes()} {
		meta, err := toml.DecodeFile(arquivo, &cfg)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return cfg, fmt.Errorf("ler %s: %w", arquivo, err)
		}
		if desconhecidas := meta.Undecoded(); len(desconhecidas) > 0 {
			return cfg, fmt.Errorf("%s: chave desconhecida %q", arquivo, desconhecidas[0].String())
		}
	}
	if cfg.Servidor.Endereco == "" {
		cfg.Servidor.Endereco = EnderecoDoUsuario(os.Getuid())
	}
	return cfg, cfg.Validar()
}

// Validar confere os valores que afetam a segurança e o funcionamento.
func (c Config) Validar() error {
	host, _, err := net.SplitHostPort(c.Servidor.Endereco)
	if err != nil {
		return fmt.Errorf("servidor.endereco: %w", err)
	}
	if !EnderecoLocal(host) {
		return fmt.Errorf("servidor.endereco: %q não é um endereço local", host)
	}
	if c.Busca.URL != "" {
		u, err := url.Parse(c.Busca.URL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil {
			return fmt.Errorf("busca.url: %q não é um endereço http válido", c.Busca.URL)
		}
	}
	if c.Orcamento.MensalUSD < 0 || c.Orcamento.PorAtividadeUSD < 0 {
		return errors.New("orcamento: os limites não podem ser negativos")
	}
	switch c.Ferramentas.Isolamento {
	case "podman", "nenhum":
	default:
		return fmt.Errorf("ferramentas.isolamento: %q (use \"podman\" ou \"nenhum\")", c.Ferramentas.Isolamento)
	}
	for _, h := range c.Ferramentas.Web.Hosts {
		if !HostValido(h) {
			return fmt.Errorf("ferramentas.web.hosts: %q não é um nome de host", h)
		}
	}
	if c.IMAP.Servidor != "" {
		if !HostValido(c.IMAP.Servidor) {
			return fmt.Errorf("imap.servidor: %q não é um nome de host", c.IMAP.Servidor)
		}
		if c.IMAP.Porta < 1 || c.IMAP.Porta > 65535 {
			return fmt.Errorf("imap.porta: %d fora do intervalo", c.IMAP.Porta)
		}
	}
	switch c.IMAP.Seguranca {
	case "tls", "starttls":
	case "nenhum":
		if c.IMAP.Servidor != "" && !EnderecoLocal(c.IMAP.Servidor) {
			return errors.New("imap.seguranca: conexão sem TLS só é aceita em localhost")
		}
	default:
		return fmt.Errorf("imap.seguranca: %q (use \"tls\" ou \"starttls\")", c.IMAP.Seguranca)
	}
	if c.IMAP.Limite < 1 || c.IMAP.Limite > 500 {
		return errors.New("imap.limite: use um valor entre 1 e 500")
	}
	switch c.Segredos.Armazenamento {
	case "arquivo", "secret-service":
	default:
		return fmt.Errorf("segredos.armazenamento: %q desconhecido", c.Segredos.Armazenamento)
	}
	if c.Acervo.IntervaloSegundos < 1 {
		return errors.New("acervo.intervalo_segundos: use ao menos 1")
	}
	return nil
}

// EnderecoLocal informa se o host se refere à própria máquina.
func EnderecoLocal(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// HostValido aceita nomes de host e endereços IP, sem esquema, porta ou caminho.
func HostValido(h string) bool {
	if h == "" || len(h) > 253 {
		return false
	}
	if net.ParseIP(h) != nil {
		return true
	}
	for _, parte := range strings.Split(h, ".") {
		if parte == "" || len(parte) > 63 || parte[0] == '-' || parte[len(parte)-1] == '-' {
			return false
		}
		for _, r := range parte {
			if !(r == '-' || r == '_' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9') {
				return false
			}
		}
	}
	return true
}

// Expandir troca o ~ inicial pelo diretório pessoal.
func Expandir(caminho string) string {
	if caminho == "~" || strings.HasPrefix(caminho, "~/") {
		if casa, err := os.UserHomeDir(); err == nil {
			return filepath.Join(casa, strings.TrimPrefix(caminho[1:], "/"))
		}
	}
	return caminho
}

// Ajustes são as opções que a interface pode alterar. Campos nulos não mudam.
type Ajustes struct {
	IMAP      *IMAP      `toml:"imap,omitempty" json:"imap,omitempty"`
	Orcamento *Orcamento `toml:"orcamento,omitempty" json:"orcamento,omitempty"`
	WebHosts  *[]string  `toml:"-" json:"web_hosts,omitempty"`
}

type arquivoAjustes struct {
	IMAP        *IMAP      `toml:"imap,omitempty"`
	Orcamento   *Orcamento `toml:"orcamento,omitempty"`
	Ferramentas *struct {
		Web *Web `toml:"web,omitempty"`
	} `toml:"ferramentas,omitempty"`
}

// GravarAjustes mescla os ajustes no ajustes.toml e devolve a configuração
// resultante. Nada é gravado se o resultado for inválido.
func GravarAjustes(c Caminhos, a Ajustes) (Config, error) {
	atual, err := Carregar(c)
	if err != nil {
		return atual, err
	}
	var arq arquivoAjustes
	if _, err := toml.DecodeFile(c.Ajustes(), &arq); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return atual, err
	}
	novo := atual
	if a.IMAP != nil {
		arq.IMAP, novo.IMAP = a.IMAP, *a.IMAP
	}
	if a.Orcamento != nil {
		arq.Orcamento, novo.Orcamento = a.Orcamento, *a.Orcamento
	}
	if a.WebHosts != nil {
		arq.Ferramentas = &struct {
			Web *Web `toml:"web,omitempty"`
		}{Web: &Web{Hosts: *a.WebHosts}}
		novo.Ferramentas.Web.Hosts = *a.WebHosts
	}
	if err := novo.Validar(); err != nil {
		return atual, err
	}
	var buf bytes.Buffer
	buf.WriteString("# Gravado pela tela de ajustes do Palantír.\n")
	if err := toml.NewEncoder(&buf).Encode(arq); err != nil {
		return atual, err
	}
	tmp := c.Ajustes() + ".tmp"
	if err := os.WriteFile(tmp, buf.Bytes(), 0o600); err != nil {
		return atual, err
	}
	if err := os.Rename(tmp, c.Ajustes()); err != nil {
		return atual, err
	}
	return novo, nil
}
