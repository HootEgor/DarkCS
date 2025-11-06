## Technology Stack

![Go](https://img.shields.io/badge/Go-1.24-blue.svg?logo=go)
![MongoDB](https://img.shields.io/badge/MongoDB-4.4-green.svg?logo=mongodb)
![Telegram](https://img.shields.io/badge/Telegram-Bot%20API-blue.svg?logo=telegram)
![OpenAI](https://img.shields.io/badge/OpenAI-GPT-blue.svg?logo=openai)
![Chi](https://img.shields.io/badge/Chi-v5-blue)

# 📌 DarkCS: Central System with AI Assistants

A centralized system with artificial intelligence that integrates customer interactions across various platforms—website, Telegram, and Instagram—using smart assistants.

## 🧠 Project Description

The main AI assistant directs client requests to the appropriate specialized assistant based on the context of the task. This approach increases service efficiency and automates key user interaction processes.

## 🔧 Functionality

*   **📁 Unified Customer Correspondence:** Consolidates customer communication from various platforms into a single database.
*   **🤖 Smart Q&A:** Answers customer questions and can promote new products during the conversation.
*   **🎁 Promotions Information:** Provides details about current special offers and promotions.
*   **🛒 Text and Voice Ordering:** Allows customers to place orders using both text and voice commands.
*   **🧮 Price Calculator:** A tool for specialists to calculate cost and retail prices.
*   **📬 Customer Feedback:** Gathers and processes feedback from clients.
*   **📦 Order Tracking:** Enables customers to track the status of their orders.

## MCP Server

The project includes a JSON-RPC 2.0 server, referred to as the MCP server. This server exposes a set of tools that can be used by AI assistants to perform various tasks. The available tools are determined by the assistant's role, allowing for specialized functionality.

### Available Tools

#### Base Tools

*   `get_products_info`: Fetches information about products.

#### Order Manager Tools

*   `create_order`: Creates an order.
*   `get_basket`: Retrieves the current shopping basket.
*   `update_user_address`: Updates a user's address.
*   `add_to_basket`: Adds products to the basket.
*   `remove_from_basket`: Removes products from the basket.
*   `get_user_info`: Retrieves user information.
*   `validate_order`: Validates an order.
*   `clear_basket`: Clears the shopping basket.

## Getting Started

1.  **Clone the repository:**
    ```bash
    git clone <repository-url>
    ```
2.  **Install Go:**
    Make sure you have Go version 1.24 or higher installed.
3.  **Install dependencies:**
    ```bash
    go mod download
    ```
4.  **Configure the application:**
    Create a `config.yml` file and fill in the necessary API keys and service configurations.
5.  **Run the application:**
    ```bash
    go run main.go
    ```
