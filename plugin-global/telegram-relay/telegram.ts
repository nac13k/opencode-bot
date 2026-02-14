export const sendTelegramReply = async (
  botToken: string,
  chatId: number,
  text: string,
): Promise<void> => {
  const response = await fetch(`https://api.telegram.org/bot${botToken}/sendMessage`, {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify({ chat_id: chatId, text }),
  });

  if (!response.ok) {
    const payload = await response.text();
    throw new Error(`Telegram relay failed (${response.status}): ${payload}`);
  }
};
