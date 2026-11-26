// Formatos devolvidos pela API do palantird.

export type EstadoAtividade = "rodando" | "esperando_voce" | "pronta" | "pausada" | "arquivada";

export interface Atividade {
  id: number;
  tipo: string;
  titulo: string;
  objetivo: string;
  estado: EstadoAtividade;
  aviso: string;
  orcamento_usd: number;
  custo_usd: number;
  atualizada_em: string;
}

export interface Confirmacao {
  id: number;
  atividade: number;
  ferramenta: string;
  argumentos: Record<string, unknown>;
  motivos: string[];
}

export interface Estado {
  versao: string;
  modelos: "ok" | "sem_chave" | "sem_rede" | "recusado";
  atividades: Atividade[];
  confirmacoes: Confirmacao[];
  gasto_mes_usd: number;
  teto_mes_usd: number;
}

export interface Detalhe {
  atividade: Atividade;
  tela: string;
  dados: unknown;
  eventos: { id: number; quando: string; origem: string; tipo: string; texto: string }[];
  confirmacoes: Confirmacao[];
}

export interface Citacao {
  marca: string;
  titulo: string;
  documento?: number;
  pagina?: number;
  url?: string;
}

export interface Quadro {
  tela: string;
  dados: any;
}

export interface Item {
  id: number;
  origem: "usuario" | "modelo" | "ferramenta" | "documento" | "sistema";
  papel: "user" | "assistant" | "tool";
  conteudo: string;
  chamadas?: { id: string; nome: string; argumentos: string }[];
  fonte?: string;
  citacoes?: Citacao[];
  quadro?: Quadro;
  bloqueado?: boolean;
}

export interface Conversa {
  forte: boolean;
  itens: Item[];
}

export interface Fonte {
  id: string;
  tipo: "documento" | "url";
  titulo: string;
  documento?: number;
  pagina?: number;
  url?: string;
  texto: string;
}

export interface Secao {
  id: number;
  titulo: string;
  estado: "pendente" | "rascunho" | "aprovada";
  paragrafos: { texto: string; fontes: string[] }[];
  comentarios: { texto: string; atendido: boolean }[];
  descartadas: number;
  aviso: string;
}

export interface Relatorio {
  planejado: boolean;
  perguntas: { texto: string; resposta: string }[];
  secoes: Secao[];
  fontes: Fonte[];
  versoes: { documento: number; quando: string }[];
}

export type Classe = "" | "urgente" | "responder" | "ler_depois" | "lixo";

export interface Mensagem {
  uid: number;
  de: string;
  email: string;
  assunto: string;
  data: string;
  trecho: string;
  classe: Classe;
  resumo: string;
  origem: string;
  etiquetada: boolean;
  concluida: boolean;
}

export interface Triagem {
  mensagens: Mensagem[];
  preferencias: { remetente: string; classe: Classe }[];
  resumo: string;
  atualizado_em: string;
  etiqueta: boolean;
}

export interface FonteEstudo {
  documento: number;
  pagina: number;
  titulo: string;
  texto: string;
}

export interface Assunto {
  id: number;
  pai: number;
  titulo: string;
  folha: boolean;
  material: number;
  cartoes: number;
  vencidos: number;
  questoes: number;
  respostas: number;
  acertos: number;
  pendente: boolean;
  aviso: string;
}

export interface Estudo {
  extraido: boolean;
  assuntos: Assunto[];
  cartoes: { id: number; assunto: number; frente: string; verso: string; fonte: FonteEstudo; novo: boolean }[];
  questoes: { id: number; assunto: number; enunciado: string; alternativas: string[]; situacao: string }[];
  painel: {
    cartoes: number;
    vencidos: number;
    questoes: number;
    respostas: number;
    acertos: number;
    hoje: number;
    proxima_em: string | null;
    mais_fraco: number;
  };
}

export interface Correcao {
  acertou: boolean;
  correta: number;
  explicacao: string;
  fonte: FonteEstudo;
}

export interface Documento {
  id: number;
  sha256: string;
  nome: string;
  tipo: string;
  tamanho: number;
  origem: string;
  importado_em: string;
  estado: "pendente" | "pronto" | "erro";
  erro?: string;
  resumo?: string;
  paginas: number;
}

export interface Colecao {
  id: number;
  nome: string;
  criada_por: "usuario" | "ia";
  documentos: number;
}

export interface Entidade {
  tipo: string;
  valor: string;
  documentos?: number;
}

export interface Trecho {
  id: number;
  documento: number;
  nome: string;
  pagina: number;
  texto: string;
}

export interface Ajustes {
  imap: {
    servidor: string;
    porta: number;
    usuario: string;
    caixa: string;
    limite: number;
    etiquetar: boolean;
    seguranca: string;
  };
  orcamento: { mensal_usd: number; por_atividade_usd: number };
  modelos: { url: string; barato: string; principal: string; forte: string; embeddings: string };
  web_hosts: string[];
  segredos: { openrouter: boolean; imap: boolean };
  caminhos: { configuracao: string; dados: string; entrada: string };
}

export interface Painel {
  gasto_mes_usd: number;
  teto_mes_usd: number;
  por_modelo: { chave: string; chamadas: number; tokens: number; custo_usd: number }[];
  por_finalidade: { chave: string; chamadas: number; tokens: number; custo_usd: number }[];
  acervo: { documentos: number; pendentes: number; com_erro: number; trechos: number; vetores: number; bytes: number };
  atividades: Record<string, number>;
}

export interface EntradaDiario {
  quando: string;
  atividade?: number;
  tipo: string;
  nome: string;
  decisao?: string;
  motivo?: string;
  custo_usd?: number;
  detalhe?: string;
}

export interface Disco {
  dispositivo: string;
  nome: string;
  sistema: string;
  montagem: string;
  tamanho: number;
  removivel: boolean;
}

export interface Discos {
  discos: Disco[];
  copia: { rodando: boolean; disco: string; arquivo?: string; erro?: string; quando: string };
}
