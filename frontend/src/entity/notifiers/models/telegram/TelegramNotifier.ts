export interface TelegramNotifier {
  botToken: string;
  targetChatId: string;
  threadId?: number;
  isHttpProxyEnabled?: boolean;
  httpProxyUrl?: string;
  hasHttpProxyUrl?: boolean;

  // temp field
  isSendToThreadEnabled?: boolean;
}
