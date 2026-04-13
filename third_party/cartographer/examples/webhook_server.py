#!/usr/bin/env python3
"""
Example Webhook Server for CMP Agents

This demonstrates how to receive context updates from CMP.
Run this server and register it as an agent webhook:

    python webhook_server.py
    cartographer agents add my-bot -t custom --webhook http://localhost:8080/webhook
    cartographer push  # Will notify this webhook
"""

from flask import Flask, request, jsonify
from datetime import datetime

app = Flask(__name__)

# Store received updates
updates = []

@app.route('/webhook', methods=['POST'])
def webhook():
    """Receive context updates from CMP"""
    try:
        payload = request.json
        
        print("\n" + "="*60)
        print(f"📥 Context Update Received at {datetime.now()}")
        print("="*60)
        print(f"Event: {payload.get('event')}")
        print(f"Context ID: {payload.get('context_id')}")
        print(f"Version: {payload.get('version')}")
        print(f"Timestamp: {payload.get('timestamp')}")
        
        changes = payload.get('changes', {})
        print(f"\nChanges:")
        print(f"  Added: {len(changes.get('added', []))} files")
        print(f"  Modified: {len(changes.get('modified', []))} files")
        print(f"  Deleted: {len(changes.get('deleted', []))} files")
        print(f"  Total: {changes.get('total_files', 0)} files")
        
        if changes.get('added'):
            print(f"\n  New files:")
            for file in changes['added'][:5]:
                print(f"    + {file}")
            if len(changes['added']) > 5:
                print(f"    ... and {len(changes['added']) - 5} more")
        
        if changes.get('modified'):
            print(f"\n  Modified files:")
            for file in changes['modified'][:5]:
                print(f"    ~ {file}")
            if len(changes['modified']) > 5:
                print(f"    ... and {len(changes['modified']) - 5} more")
        
        if changes.get('deleted'):
            print(f"\n  Deleted files:")
            for file in changes['deleted'][:5]:
                print(f"    - {file}")
            if len(changes['deleted']) > 5:
                print(f"    ... and {len(changes['deleted']) - 5} more")
        
        print("="*60)
        
        # Store update
        updates.append({
            'received_at': datetime.now().isoformat(),
            'payload': payload
        })
        
        # Here you would:
        # 1. Pull the latest context from UC
        # 2. Update your AI model's context
        # 3. Trigger any necessary reindexing
        # 4. Notify users of the update
        
        return jsonify({
            'status': 'success',
            'message': 'Context update received',
            'processed_at': datetime.now().isoformat()
        }), 200
        
    except Exception as e:
        print(f"❌ Error processing webhook: {e}")
        return jsonify({
            'status': 'error',
            'message': str(e)
        }), 500


@app.route('/health', methods=['GET'])
def health():
    """Health check endpoint"""
    return jsonify({
        'status': 'healthy',
        'updates_received': len(updates),
        'last_update': updates[-1]['received_at'] if updates else None
    })


@app.route('/updates', methods=['GET'])
def list_updates():
    """List all received updates"""
    return jsonify({
        'total': len(updates),
        'updates': updates
    })


if __name__ == '__main__':
    print("\n" + "="*60)
    print("🚀 CMP Webhook Server Starting")
    print("="*60)
    print("\nEndpoints:")
    print("  POST /webhook  - Receive context updates")
    print("  GET  /health   - Health check")
    print("  GET  /updates  - List received updates")
    print("\nTo register this webhook:")
    print("  cartographer agents add my-bot -t custom --webhook http://localhost:8080/webhook")
    print("\nTo test:")
    print("  cartographer push")
    print("="*60 + "\n")
    
    app.run(host='0.0.0.0', port=8080, debug=True)
