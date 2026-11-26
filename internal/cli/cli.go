// Package cli implementa a linha de comando do palantird.
package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"syscall"
	"time"

	"golang.org/x/term"

	"github.com/wkoch/Palantir/internal/config"
	"github.com/wkoch/Palantir/internal/daemon"
	"github.com/wkoch/Palantir/internal/ferramentas"
	"github.com/wkoch/Palantir/internal/segredos"
	"github.com/wkoch/Palantir/internal/servidor"
	"github.com/wkoch/Palantir/internal/ui"
)

// Ambiente são as entradas e saídas do comando.
type Ambiente struct {
	Versao  string
	Entrada io.Reader
	Saida   io.Writer
	Erros   io.Writer
	// Pronto, se definido, recebe o endereço depois que o servidor abre.
	Pronto func(endereco string)
}

const uso = `uso: palantird [--dir PASTA] <comando>

comandos:
  servir                     inicia o daemon (padrão)
  versao                     mostra a versão
  segredo definir NOME       guarda um segredo (openrouter ou imap), lido da entrada
  segredo remover NOME       apaga um segredo
  segredo listar             mostra quais segredos estão definidos
  importar CAMINHO...        importa arquivos ou pastas para o acervo
  exportar ARQUIVO.zip       grava originais, metadados e atividades
  verificar                  confere o hash de todos os originais
  regenerar                  refaz todos os derivados a partir dos originais
  diario [-n 50]             mostra as últimas entradas do diário
  diagnostico [--modelos]    confere se a instalação está em ordem; com --modelos,
                             faz chamadas reais e pagas (centavos) a cada nível

--dir usa uma pasta própria para configuração e dados (desenvolvimento e demonstração).
A variável PALANTIR_DIR tem o mesmo efeito.
`

type erroDeUso struct{ msg string }

func (e erroDeUso) Error() string { return e.msg }

// Executar interpreta os argumentos e devolve o código de saída.
func Executar(ctx context.Context, args []string, a Ambiente) int {
	err := executar(ctx, args, a)
	var u erroDeUso
	switch {
	case err == nil:
		return 0
	case errors.As(err, &u):
		fmt.Fprintf(a.Erros, "palantird: %s\n\n%s", u.msg, uso)
		return 2
	}
	fmt.Fprintln(a.Erros, "palantird:", err)
	return 1
}

func executar(ctx context.Context, args []string, a Ambiente) error {
	fs := flag.NewFlagSet("palantird", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	dir := fs.String("dir", os.Getenv("PALANTIR_DIR"), "")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprint(a.Saida, uso)
			return nil
		}
		return erroDeUso{err.Error()}
	}
	args = fs.Args()
	comando := "servir"
	if len(args) > 0 {
		comando, args = args[0], args[1:]
	}
	switch comando {
	case "versao", "--versao":
		fmt.Fprintln(a.Saida, a.Versao)
		return nil
	case "ajuda", "help":
		fmt.Fprint(a.Saida, uso)
		return nil
	}
	conhecidos := map[string]func(context.Context, *daemon.Daemon, []string, Ambiente) error{
		"servir": servir, "segredo": segredo, "importar": importar, "exportar": exportar,
		"verificar": verificar, "regenerar": regenerar, "diario": diario, "diagnostico": diagnostico,
	}
	f, ok := conhecidos[comando]
	if !ok {
		return erroDeUso{"comando desconhecido: " + comando}
	}
	caminhos, err := config.CaminhosPadrao(*dir)
	if err != nil {
		return err
	}
	if err := caminhos.Criar(); err != nil {
		return err
	}
	if comando == "servir" {
		destravar, err := travar(caminhos)
		if err != nil {
			return err
		}
		defer destravar()
	}
	d, err := daemon.Novo(ctx, caminhos, daemon.Opcoes{
		Log: slog.New(slog.NewTextHandler(a.Erros, nil))})
	if err != nil {
		return err
	}
	return errors.Join(f(ctx, d, args, a), d.Fechar())
}

// travar impede dois daemons sobre os mesmos dados.
func travar(c config.Caminhos) (func(), error) {
	f, err := os.OpenFile(c.Dados+"/palantird.trava", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		return nil, errors.New("já existe um palantird em execução com estes dados")
	}
	return func() { f.Close() }, nil
}

func semArgumentos(args []string) error {
	if len(args) > 0 {
		return erroDeUso{"argumentos a mais: " + strings.Join(args, " ")}
	}
	return nil
}

func servir(ctx context.Context, d *daemon.Daemon, args []string, a Ambiente) error {
	if err := semArgumentos(args); err != nil {
		return err
	}
	s := &servidor.Servidor{D: d, Versao: a.Versao, Interface: ui.Arquivos()}
	l, err := s.Escutar()
	if err != nil {
		return err
	}
	esperar, err := d.Iniciar(ctx)
	if err != nil {
		l.Close()
		return err
	}
	defer esperar()
	d.Log.Info("palantird pronto", "versao", a.Versao, "endereco", "http://"+l.Addr().String())
	if a.Pronto != nil {
		a.Pronto(l.Addr().String())
	}
	return s.Servir(ctx, l)
}

func lerSegredo(a Ambiente, nome string) (string, error) {
	if f, ok := a.Entrada.(*os.File); ok && term.IsTerminal(int(f.Fd())) {
		fmt.Fprintf(a.Erros, "Valor de %q (não aparece na tela): ", nome)
		b, err := term.ReadPassword(int(f.Fd()))
		fmt.Fprintln(a.Erros)
		return strings.TrimSpace(string(b)), err
	}
	linha, err := bufio.NewReader(io.LimitReader(a.Entrada, 8192)).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	return strings.TrimRight(linha, "\r\n"), nil
}

func segredo(ctx context.Context, d *daemon.Daemon, args []string, a Ambiente) error {
	if len(args) == 1 && args[0] == "listar" {
		for _, nome := range segredos.Conhecidos {
			v, err := d.Cofre.Ler(ctx, nome)
			if err != nil {
				return err
			}
			estado := "não definido"
			if v != "" {
				estado = "definido"
			}
			fmt.Fprintf(a.Saida, "%-12s %s\n", nome, estado)
		}
		return nil
	}
	if len(args) != 2 || (args[0] != "definir" && args[0] != "remover") {
		return erroDeUso{"use: segredo definir|remover NOME, ou segredo listar"}
	}
	nome := args[1]
	if err := segredos.Validar(nome); err != nil {
		return erroDeUso{err.Error() + " (use " + strings.Join(segredos.Conhecidos, " ou ") + ")"}
	}
	valor := ""
	if args[0] == "definir" {
		var err error
		if valor, err = lerSegredo(a, nome); err != nil {
			return err
		}
		if valor == "" {
			return errors.New("valor vazio; nada foi alterado")
		}
	}
	if err := d.DefinirSegredo(ctx, nome, valor); err != nil {
		return err
	}
	fmt.Fprintln(a.Saida, "ok")
	return nil
}

// emExecucao informa se há um daemon atendendo o endereço configurado.
func emExecucao(ctx context.Context, d *daemon.Daemon) (versao string, ok bool) {
	ctx, cancelar := context.WithTimeout(ctx, 2*time.Second)
	defer cancelar()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+d.Config().Servidor.Endereco+"/api/saude", nil)
	if err != nil {
		return "", false
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", false
	}
	defer res.Body.Close()
	var v struct {
		OK     bool   `json:"ok"`
		Versao string `json:"versao"`
	}
	if res.StatusCode != http.StatusOK || json.NewDecoder(res.Body).Decode(&v) != nil {
		return "", false
	}
	return v.Versao, v.OK
}

func importar(ctx context.Context, d *daemon.Daemon, args []string, a Ambiente) error {
	if len(args) == 0 {
		return erroDeUso{"informe ao menos um arquivo ou pasta"}
	}
	falhas := 0
	for _, caminho := range args {
		rel, err := d.Acervo.ImportarCaminho(ctx, config.Expandir(caminho), "linha de comando")
		if err != nil {
			return err
		}
		fmt.Fprintf(a.Saida, "%s: %d importado(s), %d já no acervo, %d falha(s)\n",
			caminho, rel.Importados, rel.Repetidos, len(rel.Falhas))
		for _, f := range rel.Falhas {
			fmt.Fprintln(a.Erros, "  "+f)
		}
		falhas += len(rel.Falhas)
	}
	// Com o daemon ligado, o trabalhador dele extrai o texto.
	if _, ligado := emExecucao(ctx, d); !ligado {
		fmt.Fprintln(a.Saida, "Extraindo texto…")
		if err := d.Acervo.Processar(ctx); err != nil {
			return err
		}
	}
	if falhas > 0 {
		return fmt.Errorf("%d arquivo(s) não importado(s)", falhas)
	}
	return nil
}

func exportar(ctx context.Context, d *daemon.Daemon, args []string, a Ambiente) error {
	if len(args) != 1 {
		return erroDeUso{"informe o arquivo de destino"}
	}
	destino := config.Expandir(args[0])
	// Não sobrescreve um arquivo existente.
	f, err := os.OpenFile(destino, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	err = errors.Join(d.Exportar(ctx, f), f.Close())
	if err != nil {
		os.Remove(destino)
		return err
	}
	fmt.Fprintln(a.Saida, "Exportado para", destino)
	return nil
}

func verificar(ctx context.Context, d *daemon.Daemon, args []string, a Ambiente) error {
	if err := semArgumentos(args); err != nil {
		return err
	}
	n, problemas, err := d.Acervo.Verificar(ctx)
	if err != nil {
		return err
	}
	for _, p := range problemas {
		fmt.Fprintf(a.Saida, "PROBLEMA %s (%s): %s\n", p.Nome, p.SHA256, p.Motivo)
	}
	fmt.Fprintf(a.Saida, "%d original(is) íntegro(s), %d com problema\n", n, len(problemas))
	if len(problemas) > 0 {
		return errors.New("há originais com problema")
	}
	return nil
}

func regenerar(ctx context.Context, d *daemon.Daemon, args []string, a Ambiente) error {
	if err := semArgumentos(args); err != nil {
		return err
	}
	if err := d.Acervo.Regenerar(ctx); err != nil {
		return err
	}
	nums, err := d.Acervo.Numeros(ctx)
	if err != nil {
		return err
	}
	fmt.Fprintf(a.Saida, "%d documento(s), %d trecho(s), %d com erro. Os originais não mudaram.\n",
		nums.Documentos, nums.Trechos, nums.ComErro)
	return nil
}

func diario(_ context.Context, d *daemon.Daemon, args []string, a Ambiente) error {
	fs := flag.NewFlagSet("diario", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	n := fs.Int("n", 50, "")
	if err := fs.Parse(args); err != nil {
		return erroDeUso{err.Error()}
	}
	entradas, err := d.Diario.Ultimas(*n, 0)
	if err != nil {
		return err
	}
	for i := len(entradas) - 1; i >= 0; i-- {
		e := entradas[i]
		fmt.Fprintf(a.Saida, "%s %-11s %-22s %-10s %s %s\n", e.Quando.Local().Format("02/01 15:04:05"),
			e.Tipo, e.Nome, e.Decisao, e.Motivo, e.Detalhe)
	}
	return nil
}

func diagnostico(ctx context.Context, d *daemon.Daemon, args []string, a Ambiente) error {
	testarModelos := len(args) > 0 && args[0] == "--modelos"
	if testarModelos {
		args = args[1:]
	}
	if err := semArgumentos(args); err != nil {
		return err
	}
	falhas := 0
	item := func(ok bool, nome, detalhe string) {
		marca := "ok   "
		if !ok {
			marca = "FALHA"
			falhas++
		}
		fmt.Fprintf(a.Saida, "%s %-24s %s\n", marca, nome, detalhe)
	}
	aviso := func(ok bool, nome, sim, nao string) {
		marca, detalhe := "ok   ", sim
		if !ok {
			marca, detalhe = "aviso", nao
		}
		fmt.Fprintf(a.Saida, "%s %-24s %s\n", marca, nome, detalhe)
	}
	cfg := d.Config()
	item(true, "versão", a.Versao)
	item(true, "configuração", d.Caminhos.Arquivo())

	// Executa de verdade a ferramenta de extração sobre um arquivo de teste.
	tmp, err := os.CreateTemp(d.Caminhos.Execucao, "diagnostico-*.txt")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	_, errEscrever := tmp.WriteString("diagnóstico do Palantír\n")
	if err := errors.Join(errEscrever, tmp.Close()); err != nil {
		return err
	}
	exec := &ferramentas.Processo{Config: func() config.Ferramentas { return cfg.Ferramentas }, Prazo: time.Minute}
	var r struct {
		Paginas []struct {
			Texto string `json:"texto"`
		} `json:"paginas"`
	}
	err = ferramentas.Chamar(ctx, exec, ferramentas.Execucao{Servidor: ferramentas.Extrair, Entrada: tmp.Name()},
		"extrair", map[string]any{"nome": "diagnostico.txt"}, &r)
	switch {
	case err != nil:
		item(false, "ferramentas", fmt.Sprintf("isolamento %q: %v", cfg.Ferramentas.Isolamento, err))
	case len(r.Paginas) != 1 || !strings.Contains(r.Paginas[0].Texto, "Palantír"):
		item(false, "ferramentas", "a extração de teste não devolveu o texto")
	case cfg.Ferramentas.Isolamento == "nenhum":
		aviso(false, "ferramentas", "", "funcionam, mas SEM isolamento (somente desenvolvimento)")
	default:
		item(true, "ferramentas", "contêiner "+cfg.Ferramentas.Imagem+" sem rede")
	}

	chave, err := d.Cofre.Ler(ctx, segredos.OpenRouter)
	if err != nil {
		item(false, "segredos", err.Error())
	} else {
		aviso(chave != "", "chave do OpenRouter", "definida", "não definida; só funções locais")
	}
	senha, err := d.Cofre.Ler(ctx, segredos.SenhaIMAP)
	if err != nil {
		item(false, "segredos", err.Error())
	} else {
		aviso(cfg.IMAP.Servidor != "" && senha != "", "e-mail", cfg.IMAP.Servidor, "não configurado")
	}
	versao, ligado := emExecucao(ctx, d)
	aviso(ligado, "daemon", "respondendo, versão "+versao, "não está respondendo em "+cfg.Servidor.Endereco)

	if testarModelos {
		for _, r := range d.TestarModelos(ctx) {
			detalhe := fmt.Sprintf("%s, US$ %.5f", r.Modelo, r.CustoUSD)
			if r.Erro != nil {
				detalhe = r.Erro.Error()
			}
			item(r.Erro == nil, r.Nome, detalhe)
		}
	}

	n, problemas, err := d.Acervo.Verificar(ctx)
	if err != nil {
		return err
	}
	item(len(problemas) == 0, "originais", fmt.Sprintf("%d íntegro(s), %d com problema", n, len(problemas)))
	if falhas > 0 {
		return fmt.Errorf("%d verificação(ões) falharam", falhas)
	}
	return nil
}
