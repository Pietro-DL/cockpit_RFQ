import os
from pathlib import Path

def generate_tree(dir_path: Path, prefix: str = "", ignore_hidden: bool = True):
    """
    Recursively prints a tree diagram of the directory structure.
    """
    try:
        # Excluded directories and files
        excluded = {
        "target", "__pycache__", 
        "Cargo.lock", "Cargo.toml","Commands.md", 
        "BOM_pro.exe","BOM_pro.d", "BOM_pro.pdb",
        "node_modules", "specs", ".venv", "venv", "env", "out", "bin", "lib", 
        "*.d.ps1", "docs",
        "*.pdb"
        }
        
        items = []
        for p in dir_path.iterdir():
            # Skip hidden files if ignore_hidden is True
            if ignore_hidden and p.name.startswith('.'):
                continue
            # Skip specific files/directories
            if p.name in excluded or p.name.startswith("print_tree"):
                continue
            items.append(p)
        
        # Sort so directories appear first, then files alphabetically
        items.sort(key=lambda x: (x.is_file(), x.name.lower()))
        
        count = len(items)
        for i, path in enumerate(items):
            is_last = (i == count - 1)
            
            # Determine the correct drawing characters for the tree branch
            connector = "└── " if is_last else "├── "
            
            # Print the current item
            icon = "📁 " if path.is_dir() else "📄 "
            print(f"{prefix}{connector}{icon}{path.name}")
            
            # If it's a directory, dive deeper (recursively)
            if path.is_dir():
                # Extend the prefix line down for nested items
                extension = "    " if is_last else "│   "
                generate_tree(path, prefix=prefix + extension, ignore_hidden=ignore_hidden)
                
    except PermissionError:
        print(f"{prefix}└── 🔒 [Permission Denied]")

if __name__ == "__main__":
    # Get the exact directory where THIS script file is saved
    script_directory = Path(__file__).resolve().parent
    
    print(f"📁 {script_directory.name}/")
    generate_tree(script_directory)