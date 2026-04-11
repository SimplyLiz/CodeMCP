import os
import platform
import shutil
import subprocess
import sys

TEST_DIR = "test_env"
OUTPUT_FILE = "context.xml"

def setup():
    # Clean up any previous test
    if os.path.exists(TEST_DIR):
        shutil.rmtree(TEST_DIR)
    
    # Create test structure
    os.makedirs(os.path.join(TEST_DIR, "src"))
    os.makedirs(os.path.join(TEST_DIR, "node_modules"))
    
    # Create dummy files
    with open(os.path.join(TEST_DIR, "src", "main.ts"), "w") as f:
        f.write("// Main entry point\nconsole.log('hello');")
    
    with open(os.path.join(TEST_DIR, "node_modules", "bloat.ts"), "w") as f:
        f.write("// This should be ignored\nexport const bloat = true;")

def build():
    print("Building cartographer binary...")
    result = subprocess.run(
        ["cargo", "build", "--release"],
        cwd="mapper-core/cartographer",
        capture_output=True,
        text=True
    )
    if result.returncode != 0:
        print(f"Build failed:\n{result.stderr}")
        sys.exit(1)
    print("Build successful.")

def execute():
    print("Running cartographer against test_env...")
    if platform.system() == "Windows":
        binary = os.path.join("mapper-core", "cartographer", "target", "release", "cartographer.exe")
    else:
        binary = os.path.join("mapper-core", "cartographer", "target", "release", "cartographer")
    
    result = subprocess.run(
        [binary],
        cwd=TEST_DIR,
        capture_output=True,
        text=True
    )
    if result.returncode != 0:
        print(f"Execution failed:\n{result.stderr}")
        sys.exit(1)
    print("Execution successful.")

def verify():
    output_path = os.path.join(TEST_DIR, OUTPUT_FILE)
    
    if not os.path.exists(output_path):
        print(f"❌ TEST FAILED: {OUTPUT_FILE} not generated")
        return False
    
    with open(output_path, "r", encoding="utf-8") as f:
        content = f.read()
    
    has_main = "src/main.ts" in content
    has_bloat = "node_modules/bloat.ts" in content
    
    if has_main and not has_bloat:
        print("✅ TEST PASSED: node_modules successfully ignored")
        return True
    else:
        print("❌ TEST FAILED")
        if not has_main:
            print("   - src/main.ts was NOT found (should be present)")
        if has_bloat:
            print("   - node_modules/bloat.ts WAS found (should be ignored)")
        return False

def cleanup():
    if os.path.exists(TEST_DIR):
        shutil.rmtree(TEST_DIR)

if __name__ == "__main__":
    try:
        setup()
        build()
        execute()
        success = verify()
        cleanup()
        sys.exit(0 if success else 1)
    except Exception as e:
        print(f"❌ TEST ERROR: {e}")
        cleanup()
        sys.exit(1)
